package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"novel/internal/workbench"
	"strings"
	"testing"
	"time"
)

func loginNamed(t *testing.T, s *Server, name, password string) testLogin {
	t.Helper()
	w := request(s, "POST", "/api/session", jsonBody(map[string]string{"username": name, "password": password}), testLogin{})
	v := decodeResponse[struct {
		CSRF string              `json:"csrfToken"`
		User workbench.Principal `json:"user"`
	}](t, w)
	if v.User.ID == "" || v.User.Admin {
		t.Fatal("ordinary user principal invalid")
	}
	return testLogin{Cookie: w.Result().Cookies()[0], CSRF: v.CSRF}
}
func adminUser(t *testing.T, s *Server, a testLogin, name string) workbench.User {
	t.Helper()
	return decodeResponse[workbench.User](t, request(s, "POST", "/api/admin/users", jsonBody(map[string]string{"username": name, "password": "strong-password-12", "displayName": name}), a))
}
func TestCollaborationHTTPRoleMatrixCrossProjectAndCurrentSessions(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	owner := adminUser(t, s, admin, "owner")
	editor := adminUser(t, s, admin, "editor")
	reviewer := adminUser(t, s, admin, "reviewer")
	viewer := adminUser(t, s, admin, "viewer")
	outsider := adminUser(t, s, admin, "outsider")
	sessions := map[string]testLogin{}
	for _, u := range []workbench.User{owner, editor, reviewer, viewer, outsider} {
		sessions[u.Username] = loginNamed(t, s, u.Username, "strong-password-12")
	}
	o := sessions["owner"]
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"共享作品"}`, o))
	base := "/api/projects/" + p.ID
	for _, role := range []string{"editor", "reviewer", "viewer"} {
		_ = decodeResponse[[]workbench.Member](t, request(s, "POST", base+"/members", jsonBody(map[string]string{"username": role, "role": role}), o))
	}
	c := decodeResponse[workbench.Chapter](t, request(s, "POST", base+"/chapters", `{"title":"第一章"}`, o))
	saved := decodeResponse[workbench.Revision](t, request(s, "POST", "/api/chapters/"+c.ID+"/revisions", jsonBody(workbench.SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: workbench.DocumentFromText("夜雨敲打旧窗，林川推门而入。"), Source: "author"}), o))
	for _, role := range []string{"owner", "editor", "reviewer", "viewer"} {
		if w := request(s, "GET", base, "", sessions[role]); w.Code != 200 {
			t.Fatalf("%s read denied %d", role, w.Code)
		}
	}
	for _, path := range []string{base, base + "/export", base + "/studio", base + "/research", base + "/changes", "/api/chapters/" + c.ID, "/api/revisions/" + saved.ID} {
		if w := request(s, "GET", path, "", sessions["outsider"]); w.Code != 403 {
			t.Fatalf("foreign route %s got%d", path, w.Code)
		}
	}
	if list := decodeResponse[[]workbench.Project](t, request(s, "GET", "/api/projects", "", sessions["outsider"])); len(list) != 0 {
		t.Fatal("foreign project list exposed")
	}
	for _, role := range []string{"reviewer", "viewer"} {
		if w := request(s, "POST", base+"/chapters", `{"title":"越权"}`, sessions[role]); w.Code != 403 {
			t.Fatalf("%s wrote chapter %d", role, w.Code)
		}
		if w := request(s, "POST", base+"/members", `{"username":"outsider","role":"owner"}`, sessions[role]); w.Code != 403 {
			t.Fatal("unauthorized owner promotion")
		}
	}
	if w := request(s, "POST", base+"/members", `{"username":"outsider","role":"owner"}`, sessions["editor"]); w.Code != 403 {
		t.Fatal("editor member management allowed")
	}
	comment := decodeResponse[workbench.StudioComment](t, request(s, "POST", base+"/studio/comments", jsonBody(workbench.StudioCommentInput{RevisionID: saved.ID, Quote: "夜雨", Text: "建议强化开场"}), sessions["reviewer"]))
	if comment.AuthorID != reviewer.ID {
		t.Fatal("comment lost authenticated author")
	}
	if w := request(s, "POST", base+"/studio/comments", jsonBody(workbench.StudioCommentInput{RevisionID: saved.ID, Quote: "夜雨", Text: "越权批注"}), sessions["viewer"]); w.Code != 403 {
		t.Fatal("viewer commented")
	}
	// Two authenticated editors start at the same base; only the first commit wins.
	first := decodeResponse[workbench.Revision](t, request(s, "POST", "/api/chapters/"+c.ID+"/revisions", jsonBody(workbench.SaveInput{ExpectedCurrentRevisionID: saved.ID, Document: workbench.DocumentFromText("编辑者的新版本。")}), sessions["editor"]))
	if first.ID == saved.ID {
		t.Fatal("save did not create revision")
	}
	if w := request(s, "POST", "/api/chapters/"+c.ID+"/revisions", jsonBody(workbench.SaveInput{ExpectedCurrentRevisionID: saved.ID, Document: workbench.DocumentFromText("所有者旧页。")}), o); w.Code != 409 {
		t.Fatal("shared write silently overwrote latest")
	}
	for _, path := range []string{"/api/metrics", "/api/provider-configs", "/api/mcp-configs", "/api/mcp-call-logs", "/api/admin/users"} {
		if w := request(s, "GET", path, "", sessions["editor"]); w.Code != 403 {
			t.Fatalf("sensitive global %s accessible %d", path, w.Code)
		}
	}
	if w := request(s, "DELETE", base+"/members/"+editor.ID, "", o); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(s, "GET", base, "", sessions["editor"]); w.Code != 403 {
		t.Fatal("existing session retained removed role")
	}
	_ = outsider
}

func TestCollaborationHTTPInviteRegistrationAndAuthenticatedSSE(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"邀请与通知"}`, admin))
	base := "/api/projects/" + p.ID
	invite := decodeResponse[workbench.Invitation](t, request(s, "POST", base+"/invitations", `{"role":"reviewer"}`, admin))
	user := decodeResponse[workbench.User](t, request(s, "POST", "/api/invitations/accept", jsonBody(map[string]string{"token": invite.Token, "username": "reader", "password": "strong-password-12", "displayName": "审阅者"}), testLogin{}))
	session := loginNamed(t, s, "reader", "strong-password-12")
	access := decodeResponse[struct {
		Role string `json:"role"`
	}](t, request(s, "GET", base+"/access", "", session))
	if access.Role != "reviewer" {
		t.Fatal("invitation role lost")
	}
	if w := request(s, "POST", "/api/invitations/accept", jsonBody(map[string]string{"token": invite.Token, "username": "reader2", "password": "strong-password-12", "displayName": "伪用"}), testLogin{}); w.Code == 200 {
		t.Fatal("invite reused")
	}
	if w := request(s, "GET", base+"/invitations", "", session); w.Code != 403 {
		t.Fatal("reviewer reads invitation secrets")
	}
	presence := decodeResponse[[]workbench.Presence](t, request(s, "POST", base+"/presence", `{}`, session))
	if len(presence) != 1 || presence[0].UserID != user.ID {
		t.Fatal("real presence not saved")
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+base+"/changes?after=0", nil)
	req.AddCookie(session.Cookie)
	res, e := ts.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatal("SSE auth failed", res.Status)
	}
	c := decodeResponse[workbench.Chapter](t, request(s, "POST", base+"/chapters", `{"title":"实时新增"}`, admin))
	scan := bufio.NewScanner(res.Body)
	seen := false
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "data: ") {
			var event workbench.CollaborationEvent
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil && event.ObjectID == c.ID {
				seen = true
				break
			}
		}
	}
	if !seen {
		t.Fatal("persisted chapter change not delivered", scan.Err())
	}
	cancel()
	// Session cookie is opaque, never the stored token hash.
	var hash string
	if e = s.Service.DB.QueryRow(context.Background(), "SELECT id_hash FROM wb_sessions WHERE user_id=$1", user.ID).Scan(&hash); e != nil || hash == session.Cookie.Value {
		t.Fatal("session token stored plaintext")
	}
}
