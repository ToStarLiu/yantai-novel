package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"novel/internal/workbench"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCopilotHTTPPrivateRoleAccessAndEventsReplay(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	viewer := adminUser(t, s, admin, "copilot-viewer")
	other := adminUser(t, s, admin, "copilot-other")
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"对话权限作品"}`, admin))
	base := "/api/projects/" + p.ID
	_ = decodeResponse[[]workbench.Member](t, request(s, "POST", base+"/members", jsonBody(map[string]string{"username": viewer.Username, "role": "viewer"}), admin))
	c := decodeResponse[workbench.Chapter](t, request(s, "POST", base+"/chapters", `{"title":"桥下"}`, admin))
	vLogin := loginNamed(t, s, viewer.Username, "strong-password-12")
	oLogin := loginNamed(t, s, other.Username, "strong-password-12")
	session := decodeResponse[workbench.CopilotSession](t, request(s, "POST", base+"/copilot/sessions", jsonBody(workbench.CopilotSessionInput{ChapterID: c.ID}), vLogin))
	if w := request(s, "GET", "/api/copilot/sessions/"+session.ID, "", admin); w.Code != 403 {
		t.Fatal("administrator read another user's private conversation", w.Code)
	}
	if w := request(s, "GET", "/api/copilot/sessions/"+session.ID, "", oLogin); w.Code != 403 {
		t.Fatal("outsider read private conversation", w.Code)
	}
	turn := decodeResponse[workbench.CopilotTurn](t, request(s, "POST", "/api/copilot/sessions/"+session.ID+"/turns", `{"intentId":"private-turn","text":"讨论而不改稿"}`, vLogin))
	events := decodeResponse[[]workbench.RunEvent](t, request(s, "GET", "/api/copilot/turns/"+turn.ID+"/events?format=json", "", vLogin))
	if len(events) != 1 || events[0].Kind != "status" {
		t.Fatal("missing pending event", events)
	}
	if w := request(s, "POST", "/api/copilot/turns/"+turn.ID+"/cancel", `{}`, oLogin); w.Code != 403 {
		t.Fatal("outsider cancelled turn", w.Code)
	}
	_ = decodeResponse[workbench.CopilotTurn](t, request(s, "POST", "/api/copilot/turns/"+turn.ID+"/cancel", `{}`, vLogin))
	if w := request(s, "GET", "/api/copilot/turns/"+turn.ID+"/attempts", "", vLogin); w.Code != 200 {
		t.Fatal("viewer cannot read own accounting", w.Code)
	}
	// Current membership is checked on private session reads, not only creation.
	if w := request(s, "DELETE", base+"/members/"+viewer.ID, "", admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(s, "GET", "/api/copilot/sessions/"+session.ID, "", vLogin); w.Code != 403 {
		t.Fatal("revoked viewer retained conversation access", w.Code)
	}
}

func TestCopilotSSEPersistsCursorAndDoesNotOwnTaskLifetime(t *testing.T) {
	s := testServer(t)
	login := loginForTest(t, s)
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"对话流"}`, login))
	c := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+p.ID+"/chapters", `{"title":"桥下"}`, login))
	session := decodeResponse[workbench.CopilotSession](t, request(s, "POST", "/api/projects/"+p.ID+"/copilot/sessions", jsonBody(workbench.CopilotSessionInput{ChapterID: c.ID}), login))
	turn := decodeResponse[workbench.CopilotTurn](t, request(s, "POST", "/api/copilot/sessions/"+session.ID+"/turns", `{"intentId":"stream-private-turn","text":"继续讨论"}`, login))
	host := httptest.NewServer(s.Handler())
	defer host.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", host.URL+"/api/copilot/turns/"+turn.ID+"/events", nil)
	req.AddCookie(login.Cookie)
	response, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	reader := bufio.NewScanner(response.Body)
	found := false
	var event workbench.RunEvent
	for reader.Scan() {
		line := reader.Text()
		if strings.HasPrefix(line, "data: ") {
			if e = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); e != nil {
				t.Fatal(e)
			}
			found = true
			break
		}
	}
	_ = response.Body.Close()
	if !found || event.Kind != "status" {
		t.Fatal("real SSE missing event", event, reader.Err())
	}
	persisted, e := s.Service.GetCopilotTurn(context.Background(), turn.ID)
	if e != nil || persisted.Status != "Pending" {
		t.Fatal("SSE disconnect changed task", persisted, e)
	}
	replay := decodeResponse[[]workbench.RunEvent](t, request(s, "GET", "/api/copilot/turns/"+turn.ID+"/events?format=json&after="+strconvI64(event.ID), "", login))
	if len(replay) != 0 {
		t.Fatal("cursor duplicated event")
	}
}
func strconvI64(v int64) string { raw, _ := json.Marshal(v); return string(raw) }

func TestBookCopilotHTTPZeroChapterScopeAndExplicitAdoption(t *testing.T) {
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	entry, _ := filepath.Abs("../../harness/dist/main.js")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err := os.Stat(entry); err != nil {
		t.Skip("Build Pi harness")
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	s := testServer(t)
	admin := loginForTest(t, s)
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"零章节全书规划"}`, admin))
	base := "/api/projects/" + p.ID
	session := decodeResponse[workbench.CopilotSession](t, request(s, "POST", base+"/copilot/sessions", `{"scope":"book"}`, admin))
	if session.Scope != "book" || session.ChapterID != "" {
		t.Fatal("book scope not persisted", session)
	}
	books := decodeResponse[[]workbench.CopilotSession](t, request(s, "GET", base+"/copilot/sessions?scope=book", "", admin))
	chapters := decodeResponse[[]workbench.CopilotSession](t, request(s, "GET", base+"/copilot/sessions?scope=chapter", "", admin))
	if len(books) != 1 || len(chapters) != 0 {
		t.Fatal("session scopes mixed", books, chapters)
	}
	if w := request(s, "POST", base+"/copilot/sessions", `{"scope":"book","chapterId":"fake"}`, admin); w.Code != 400 {
		t.Fatal("book accepted a fake chapter", w.Code, w.Body.String())
	}
	turnPath := "/api/copilot/sessions/" + session.ID + "/turns"
	if w := request(s, "POST", turnPath, `{"intentId":"wrong-scope","text":"切换到章节","scope":"chapter"}`, admin); w.Code != 400 {
		t.Fatal("changed existing scope", w.Code)
	}
	turn := decodeResponse[workbench.CopilotTurn](t, request(s, "POST", turnPath, `{"intentId":"book-planning-http","text":"一起规划这本书"}`, admin))
	if turn.Input.Scope != "book" || turn.Input.ChapterID != "" {
		t.Fatal("turn lost scope", turn)
	}
	turn.Status = "Running"
	turn.LeaseOwner = "book-http-test-worker"
	if _, err := s.Service.DB.Exec(context.Background(), "UPDATE wb_copilot_turns SET status='Running',lease_owner=$2,lease_until=now()+interval '30 seconds' WHERE id=$1", turn.ID, turn.LeaseOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.Service.ExecuteCopilotTurn(context.Background(), turn); err != nil {
		t.Fatal(err)
	}
	completed := decodeResponse[workbench.CopilotTurn](t, request(s, "GET", "/api/copilot/turns/"+turn.ID, "", admin))
	if completed.Status != "Succeeded" {
		t.Fatal("Pi book request failed", completed.Error)
	}
	var artifacts []workbench.CopilotArtifact
	if err := json.Unmarshal(completed.Artifacts, &artifacts); err != nil {
		t.Fatal(err)
	}
	var proposal workbench.CopilotArtifact
	for _, artifact := range artifacts {
		if artifact.Kind == "book-plan-proposal" {
			proposal = artifact
		}
	}
	if proposal.ProposalID == "" {
		t.Fatal("plan candidate missing", completed.Artifacts)
	}
	view := decodeResponse[workbench.StudioView](t, request(s, "GET", base+"/studio", "", admin))
	if view.Brief.Data.PlotPlan != "" {
		t.Fatal("plan automatically adopted")
	}
	apply := "/api/copilot/book-plans/" + proposal.ProposalID + "/apply"
	if w := request(s, "POST", apply, `{"expectedVersion":0}`, testLogin{Cookie: admin.Cookie}); w.Code != 403 {
		t.Fatal("adoption ignored CSRF", w.Code)
	}
	saved := decodeResponse[workbench.StudioDocument[workbench.StoryBrief]](t, request(s, "POST", apply, `{"expectedVersion":0}`, admin))
	if saved.Version != 1 || saved.Data.PlotPlan != proposal.Text {
		t.Fatal("explicit plan adoption failed", saved)
	}
	detail := decodeResponse[workbench.ProjectDetail](t, request(s, "GET", base, "", admin))
	if len(detail.Chapters) != 0 {
		t.Fatal("planning created chapters", detail.Chapters)
	}
}
