package server

import (
	"context"
	"novel/internal/workbench"
	"strings"
	"testing"
)

func TestAuthorTaskContextHTTPOwnerOnlyAndSharedRunRemainsPublic(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	user := adminUser(t, s, admin, "task-owner")
	viewer := adminUser(t, s, admin, "task-viewer")
	ownerLogin := loginNamed(t, s, user.Username, "strong-password-12")
	viewerLogin := loginNamed(t, s, viewer.Username, "strong-password-12")
	project := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"私有作者任务"}`, ownerLogin))
	_ = decodeResponse[[]workbench.Member](t, request(s, "POST", "/api/projects/"+project.ID+"/members", jsonBody(map[string]string{"username": viewer.Username, "role": "viewer"}), ownerLogin))
	chapter := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+project.ID+"/chapters", `{"title":"第一章"}`, ownerLogin))
	note := decodeResponse[workbench.AuthorMemory](t, request(s, "POST", "/api/author-memories", `{"kind":"voice","title":"私人标题","content":"PRIVATE-HTTP-TASK: 句子克制，不复述情绪。"}`, ownerLogin))
	run := decodeResponse[workbench.Run](t, request(s, "POST", "/api/chapters/"+chapter.ID+"/runs", jsonBody(workbench.RunRequest{IntentID: "owned-plan", TaskType: "planning"}), ownerLogin))
	if run.Input.OwnerID != user.ID {
		t.Fatal("task owner not frozen")
	}
	path := "/api/runs/" + run.ID + "/author-context"
	private := decodeResponse[workbench.AuthorTaskContext](t, request(s, "GET", path, "", ownerLogin))
	if private.OwnerID != user.ID || len(private.Memory.Items) != 1 || private.Memory.Items[0].Content != note.Content || !private.Current {
		t.Fatal("private context not visible to owner", private)
	}
	for _, other := range []testLogin{admin, viewerLogin} {
		if w := request(s, "GET", path, "", other); w.Code != 404 || strings.Contains(w.Body.String(), note.Content) {
			t.Fatal("another account accessed private task context", w.Code, w.Body.String())
		}
	}
	if w := request(s, "GET", path, "", testLogin{}); w.Code != 401 {
		t.Fatal("anonymous task context", w.Code)
	}
	for _, other := range []testLogin{admin, ownerLogin, viewerLogin} {
		w := request(s, "GET", "/api/runs/"+run.ID, "", other)
		if w.Code != 200 || strings.Contains(w.Body.String(), note.Content) || strings.Contains(w.Body.String(), note.Title) {
			t.Fatal("shared run leaked preferences", w.Code, w.Body.String())
		}
	}
	ignored := decodeResponse[workbench.Run](t, request(s, "POST", "/api/chapters/"+chapter.ID+"/runs", jsonBody(workbench.RunRequest{IntentID: "ignore-plan", TaskType: "planning", IgnoreMemory: true}), ownerLogin))
	ignoredContext := decodeResponse[workbench.AuthorTaskContext](t, request(s, "GET", "/api/runs/"+ignored.ID+"/author-context", "", ownerLogin))
	if !ignoredContext.IgnoreMemory || len(ignoredContext.Memory.Items) > 0 {
		t.Fatal("request ignore flag lost", ignoredContext)
	}
	if w := request(s, "DELETE", "/api/author-memories/"+note.ID+"?expectedVersion=1", "", ownerLogin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	stale := decodeResponse[workbench.AuthorTaskContext](t, request(s, "GET", path, "", ownerLogin))
	if stale.Current {
		t.Fatal("forgotten context advertised current")
	}
	// Legacy unowned work does not gain an environment-admin preference snapshot.
	legacy, err := s.Service.SubmitRun(context.Background(), chapter.ID, workbench.RunRequest{IntentID: "unowned-plan", TaskType: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(s, "GET", "/api/runs/"+legacy.ID+"/author-context", "", admin); w.Code != 404 {
		t.Fatal("legacy task invented a private author", w.Code, w.Body.String())
	}
}
