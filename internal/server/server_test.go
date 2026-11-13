package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"novel/internal/job"
	"novel/internal/workbench"
)

func jsonBody(v any) string { raw, _ := json.Marshal(v); return string(raw) }

func TestWorkbenchHTTPRevisionImportExportFlow(t *testing.T) {
	s := testServer(t)
	session := loginForTest(t, s)
	project := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"断桥铜印","description":"原创测试","genre":"悬疑"}`, session))
	_ = decodeResponse[workbench.Setting](t, request(s, "POST", "/api/projects/"+project.ID+"/settings", `{"kind":"character","name":"林川","content":"青年旅人"}`, session))
	chapter := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+project.ID+"/chapters", `{"title":"第一章","volume":"第一卷"}`, session))
	initialID := chapter.CurrentRevisionID
	save := workbench.SaveInput{ExpectedCurrentRevisionID: initialID, Document: workbench.DocumentFromText("林川的左臂仍在疼痛。\n铜印留在胸前内袋。")}
	first := decodeResponse[workbench.Revision](t, request(s, "POST", "/api/chapters/"+chapter.ID+"/revisions", jsonBody(save), session))
	if !strings.Contains(first.PlainText, "左臂") {
		t.Fatal("Chinese text not persisted")
	}
	conflict := request(s, "POST", "/api/chapters/"+chapter.ID+"/revisions", jsonBody(save), session)
	if conflict.Code != 409 {
		t.Fatalf("save conflict %d %s", conflict.Code, conflict.Body.String())
	}
	var current struct {
		ID string `json:"currentRevisionId"`
	}
	_ = json.Unmarshal(conflict.Body.Bytes(), &current)
	if current.ID != first.ID {
		t.Fatal("conflict missing current revision")
	}
	history := decodeResponse[[]workbench.Revision](t, request(s, "GET", "/api/chapters/"+chapter.ID+"/revisions", "", session))
	if len(history) != 2 {
		t.Fatalf("history %d", len(history))
	}
	restored := decodeResponse[workbench.Revision](t, request(s, "POST", "/api/chapters/"+chapter.ID+"/restore", jsonBody(map[string]string{"revisionId": initialID, "expectedCurrentRevisionId": first.ID}), session))
	if restored.ID == initialID || restored.PlainText != "" {
		t.Fatal("restore overwrote history")
	}
	save.ExpectedCurrentRevisionID = restored.ID
	final := decodeResponse[workbench.Revision](t, request(s, "POST", "/api/chapters/"+chapter.ID+"/revisions", jsonBody(save), session))
	_ = decodeResponse[workbench.Impact](t, request(s, "POST", "/api/chapters/"+chapter.ID+"/finalize", jsonBody(map[string]string{"revisionId": final.ID}), session))
	archive := decodeResponse[workbench.ProjectPackage](t, request(s, "GET", "/api/projects/"+project.ID+"/export", "", session))
	if len(archive.Revisions) != 4 {
		t.Fatalf("archive revisions %d", len(archive.Revisions))
	}
	imported := decodeResponse[workbench.ProjectDetail](t, request(s, "POST", "/api/import", jsonBody(map[string]any{"package": archive}), session))
	if imported.Project.ID == project.ID || len(imported.Chapters) != 1 {
		t.Fatal("import did not create isolated project")
	}
	importedChapter := decodeResponse[workbench.Chapter](t, request(s, "GET", "/api/chapters/"+imported.Chapters[0].ID, "", session))
	if importedChapter.Revision.PlainText != final.PlainText || importedChapter.FinalRevisionID != importedChapter.CurrentRevisionID {
		t.Fatal("import lost current or finalized text")
	}
	markdown := request(s, "GET", "/api/projects/"+imported.Project.ID+"/markdown", "", session)
	if markdown.Code != 200 || !strings.Contains(markdown.Body.String(), "铜印留在胸前内袋。") {
		t.Fatalf("markdown export %d", markdown.Code)
	}
	preview := decodeResponse[[]workbench.ImportChapter](t, request(s, "POST", "/api/import/preview", `{"text":"第1章 再起\n风雨不停。\n第2章 灯下\n木箱未开。"}`, session))
	if len(preview) != 2 {
		t.Fatal("TXT preview wrong")
	}
	after := decodeResponse[workbench.ProjectDetail](t, request(s, "POST", "/api/projects/"+imported.Project.ID+"/import", jsonBody(map[string]any{"chapters": preview}), session))
	if len(after.Chapters) != 3 {
		t.Fatal("confirmed import wrong")
	}
	archive.Revisions[0].PlainText = "tampered"
	if w := request(s, "POST", "/api/import", jsonBody(map[string]any{"package": archive}), session); w.Code != 400 {
		t.Fatalf("tampered archive HTTP %d", w.Code)
	}
	projects := decodeResponse[[]workbench.Project](t, request(s, "GET", "/api/projects", "", session))
	if len(projects) != 2 {
		t.Fatal("failed import left partial project")
	}
}

func TestHTTPRejectsCrossChapterSourceAndMalformedInput(t *testing.T) {
	s := testServer(t)
	session := loginForTest(t, s)
	a := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"A"}`, session))
	b := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"B"}`, session))
	ca := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+a.ID+"/chapters", `{"title":"A1"}`, session))
	cb := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+b.ID+"/chapters", `{"title":"B1"}`, session))
	if w := request(s, "POST", "/api/chapters/"+ca.ID+"/finalize", jsonBody(map[string]string{"revisionId": cb.CurrentRevisionID}), session); w.Code != 404 {
		t.Fatalf("foreign finalize %d", w.Code)
	}
	if w := request(s, "POST", "/api/chapters/"+ca.ID+"/restore", jsonBody(map[string]string{"revisionId": cb.CurrentRevisionID, "expectedCurrentRevisionId": ca.CurrentRevisionID}), session); w.Code != 404 {
		t.Fatalf("foreign restore %d", w.Code)
	}
	for _, body := range []string{`{"title":"valid","extra":true}`, `{"title":"valid"} {"title":"second"}`, `not-json`} {
		if w := request(s, "POST", "/api/projects", body, session); w.Code != 400 {
			t.Fatalf("invalid body %q status %d", body, w.Code)
		}
	}
}

func TestSSEDisconnectLeavesPersistedTaskAndCanReplay(t *testing.T) {
	s := testServer(t)
	session := loginForTest(t, s)
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"事件流"}`, session))
	c := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+p.ID+"/chapters", `{"title":"第一章"}`, session))
	r := decodeResponse[workbench.Run](t, request(s, "POST", "/api/chapters/"+c.ID+"/runs", jsonBody(workbench.RunRequest{IntentID: job.NewID(), TaskType: "review", InputRevisionID: c.CurrentRevisionID}), session))
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	readEvents := func(after string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/runs/"+r.ID+"/events", nil)
		req.AddCookie(session.Cookie)
		if after != "" {
			req.Header.Set("Last-Event-ID", after)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("SSE status %d", res.StatusCode)
		}
		buf := make([]byte, 4096)
		n, err := res.Body.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		return string(buf[:n])
	}
	first := readEvents("")
	if !strings.Contains(first, "Pending") {
		t.Fatalf("persisted event absent %q", first)
	}
	events, err := s.Service.Events(context.Background(), r.ID, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("events %#v %v", events, err)
	}
	replay := readEvents("0")
	if !strings.Contains(replay, "Pending") {
		t.Fatal("reconnect lost persisted event")
	}
	current, err := s.Service.GetRun(context.Background(), r.ID)
	if err != nil || current.Status != "Pending" {
		t.Fatalf("disconnect changed task %#v %v", current, err)
	}
	var cancelled bool
	if err = s.Service.DB.QueryRow(context.Background(), "SELECT cancel_requested FROM wb_runs WHERE id=$1", r.ID).Scan(&cancelled); err != nil || cancelled {
		t.Fatal("disconnect requested task cancellation")
	}
}
