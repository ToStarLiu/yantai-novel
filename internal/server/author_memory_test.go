package server

import (
	"context"
	"novel/internal/workbench"
	"strings"
	"testing"
)

func TestAuthorMemoryHTTPPersonalAuthorizationAndCAS(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	user := adminUser(t, s, admin, "memory-author")
	author := loginNamed(t, s, user.Username, "strong-password-12")
	if w := request(s, "GET", "/api/author-memories", "", testLogin{}); w.Code != 401 {
		t.Fatal("anonymous private memory route", w.Code)
	}
	settings := decodeResponse[workbench.AuthorMemorySettings](t, request(s, "GET", "/api/author-memory-settings", "", author))
	if !settings.UseMemory || !settings.SuggestMemories || settings.Version != 0 {
		t.Fatal(settings)
	}
	in := workbench.AuthorMemoryInput{Scope: "author", Kind: "voice", Title: "对白偏好", Content: "PRIVATE-HTTP: 对话简短，让动作承担潜台词。"}
	v := decodeResponse[workbench.AuthorMemory](t, request(s, "POST", "/api/author-memories", jsonBody(in), author))
	if v.OwnerID != user.ID || v.Status != "active" || v.Version != 1 {
		t.Fatal(v)
	}
	if w := request(s, "POST", "/api/author-memories", jsonBody(in), testLogin{Cookie: author.Cookie}); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	list := decodeResponse[workbench.AuthorMemoryList](t, request(s, "GET", "/api/author-memories", "", author))
	if len(list.Items) != 1 || list.Version != 1 || list.Settings.Version != 1 {
		t.Fatal(list)
	}
	adminList := decodeResponse[workbench.AuthorMemoryList](t, request(s, "GET", "/api/author-memories", "", admin))
	if len(adminList.Items) != 0 {
		t.Fatal("administrator enumerated author memories")
	}
	path := "/api/author-memories/" + v.ID
	for _, method := range []string{"PUT", "DELETE"} {
		if w := request(s, method, path, `{"expectedVersion":1}`, admin); w.Code != 404 {
			t.Fatal("administrator mutated another author", method, w.Code, w.Body.String())
		}
	}
	if w := request(s, "PUT", path, `{"expectedVersion":0,"content":"旧窗口覆盖"}`, author); w.Code != 409 {
		t.Fatal("stale overwrite accepted", w.Code, w.Body.String())
	}
	v = decodeResponse[workbench.AuthorMemory](t, request(s, "PUT", path, `{"expectedVersion":1,"status":"disabled"}`, author))
	if v.Status != "disabled" || v.Version != 2 {
		t.Fatal(v)
	}
	if w := request(s, "DELETE", path, `{"expectedVersion":1}`, author); w.Code != 409 {
		t.Fatal("stale forget accepted", w.Code)
	}
	settings = decodeResponse[workbench.AuthorMemorySettings](t, request(s, "GET", "/api/author-memory-settings", "", author))
	settings = decodeResponse[workbench.AuthorMemorySettings](t, request(s, "PUT", "/api/author-memory-settings", jsonBody(workbench.AuthorMemorySettingsUpdate{ExpectedVersion: settings.Version, UseMemory: false, SuggestMemories: false}), author))
	if settings.UseMemory || settings.SuggestMemories {
		t.Fatal(settings)
	}
	if w := request(s, "PUT", "/api/author-memory-settings", `{"expectedVersion":0,"useMemory":true,"suggestMemories":true}`, author); w.Code != 409 {
		t.Fatal("stale settings accepted", w.Code)
	}
	for _, version := range []string{"0", "-1", "invalid"} {
		if w := request(s, "DELETE", path+"?expectedVersion="+version, "", author); w.Code != 400 {
			t.Fatal("invalid delete version accepted", version, w.Code)
		}
	}
	if w := request(s, "DELETE", path+"?expectedVersion=1", "", author); w.Code != 409 {
		t.Fatal("stale query delete accepted", w.Code)
	}
	if w := request(s, "DELETE", path+"?expectedVersion=2", "", author); w.Code != 200 {
		t.Fatal("query version delete failed", w.Code, w.Body.String())
	}
	list = decodeResponse[workbench.AuthorMemoryList](t, request(s, "GET", "/api/author-memories", "", author))
	if len(list.Items) != 0 {
		t.Fatal("forgotten memory still visible")
	}
}

func TestAuthorMemoryHTTPBookMembershipAndPortableImport(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	user := adminUser(t, s, admin, "book-author")
	author := loginNamed(t, s, user.Username, "strong-password-12")
	other := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"他人作品"}`, admin))
	book := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"自己的作品"}`, author))
	in := workbench.AuthorMemoryInput{Scope: "book", ProjectID: other.ID, Kind: "workflow", Title: "个人习惯", Content: "PRIVATE-BOOK-HTTP: 每章先列行动再写正文。"}
	if w := request(s, "POST", "/api/author-memories", jsonBody(in), author); w.Code != 403 {
		t.Fatal("private book memory accepted inaccessible book", w.Code)
	}
	if w := request(s, "GET", "/api/author-memories?projectId="+other.ID, "", author); w.Code != 403 {
		t.Fatal("foreign book filter bypassed membership", w.Code)
	}
	in.ProjectID = book.ID
	decodeResponse[workbench.AuthorMemory](t, request(s, "POST", "/api/author-memories", jsonBody(in), author))
	w := request(s, "GET", "/api/author-memories/export", "", author)
	exported := decodeResponse[workbench.AuthorMemoryExport](t, w)
	if exported.FormatVersion != 1 || len(exported.Items) != 1 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("invalid portable export", w.Header(), exported)
	}
	result := decodeResponse[workbench.AuthorMemoryImportResult](t, request(s, "POST", "/api/author-memories/import", jsonBody(workbench.AuthorMemoryImport{Items: exported.Items}), author))
	if result.Skipped != 1 || result.Imported != 0 {
		t.Fatal("import should deduplicate", result)
	}
	bad := `{"items":[{"title":"假来源","content":"规则","sourceTurnId":"someone-else","sourceQuote":"摘录"}]}`
	if w := request(s, "POST", "/api/author-memories/import", bad, author); w.Code != 400 {
		t.Fatal("import accepted forged evidence", w.Code, w.Body.String())
	}
	if w := request(s, "POST", "/api/author-memories/import", `{"items":[{"title":"脚本","content":"规则","command":"rm -rf"}]}`, author); w.Code != 400 {
		t.Fatal("unknown executable field accepted", w.Code)
	}
	shared := request(s, "GET", "/api/projects/"+book.ID+"/export", "", author)
	if shared.Code != 200 || strings.Contains(shared.Body.String(), "PRIVATE-BOOK-HTTP") {
		t.Fatal("shared export leaked author-private memory", shared.Code, shared.Body.String())
	}
	if w := request(s, "GET", "/api/author-memories?status=unknown", "", author); w.Code != 400 {
		t.Fatal("invalid filter ignored", w.Code)
	}
}

func TestAuthorMemoryHTTPForgottenCandidateReportsStaleOnReadAndList(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"作者记忆候选状态"}`, admin))
	c := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+p.ID+"/chapters", `{"title":"雨夜重逢"}`, admin))
	v := decodeResponse[workbench.AuthorMemory](t, request(s, "POST", "/api/author-memories", `{"kind":"conversation","title":"雨夜重逢","content":"雨夜重逢先用旧伞确认身份。"}`, admin))
	ctx := workbench.WithPrincipal(context.Background(), workbench.Principal{ID: "admin", Admin: true})
	scope, err := s.Service.NewCopilotToolContext(ctx, "http-memory-candidate", p.ID, c.ID, c.CurrentRevisionID, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Service.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"text":"她收起旧伞，望向对方。","mode":"chapter"}`))
	if err != nil {
		t.Fatal(err)
	}
	runPath := "/api/runs/" + result.Artifact.RunID
	before := decodeResponse[workbench.Run](t, request(s, "GET", runPath, "", admin))
	if before.Stale {
		t.Fatal("fresh candidate HTTP state is stale")
	}
	if w := request(s, "DELETE", "/api/author-memories/"+v.ID+"?expectedVersion=1", "", admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	after := decodeResponse[workbench.Run](t, request(s, "GET", runPath, "", admin))
	if !after.Stale {
		t.Fatal("HTTP candidate getter advertised adoptability after forget")
	}
	list := decodeResponse[[]workbench.Run](t, request(s, "GET", "/api/projects/"+p.ID+"/runs", "", admin))
	if len(list) != 1 || !list[0].Stale {
		t.Fatal("HTTP candidate list advertised adoptability after forget", list)
	}
	save := workbench.SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: workbench.DocumentFromText(after.Output.Text), Source: "ai", RunID: after.ID}
	if w := request(s, "POST", "/api/chapters/"+c.ID+"/revisions", jsonBody(save), admin); w.Code != 409 {
		t.Fatal("adopt request accepted forgotten preference candidate", w.Code, w.Body.String())
	}
}
