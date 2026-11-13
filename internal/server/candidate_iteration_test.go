package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"novel/internal/workbench"
)

func TestCandidateIterationHTTPBindsExactCandidateAndRejectsOtherAuthors(t *testing.T) {
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
	p := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"候选迭代HTTP"}`, admin))
	c := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+p.ID+"/chapters", `{"title":"桥边"}`, admin))
	r := decodeResponse[workbench.Revision](t, request(s, "POST", "/api/chapters/"+c.ID+"/revisions", jsonBody(map[string]any{"expectedCurrentRevisionId": c.CurrentRevisionID, "document": workbench.DocumentFromText("林川左臂仍然受伤。"), "source": "author"}), admin))
	c.CurrentRevisionID = r.ID
	session := decodeResponse[workbench.CopilotSession](t, request(s, "POST", "/api/projects/"+p.ID+"/copilot/sessions", jsonBody(workbench.CopilotSessionInput{ChapterID: c.ID}), admin))
	path := "/api/copilot/sessions/" + session.ID + "/turns"
	runTurn := func(turn workbench.CopilotTurn) workbench.CopilotTurn {
		t.Helper()
		turn.LeaseOwner = "candidate-http-worker"
		turn.Status = "Running"
		if _, err := s.Service.DB.Exec(context.Background(), "UPDATE wb_copilot_turns SET status='Running',lease_owner=$2,lease_until=now()+interval '30 seconds' WHERE id=$1", turn.ID, turn.LeaseOwner); err != nil {
			t.Fatal(err)
		}
		if err := s.Service.ExecuteCopilotTurn(context.Background(), turn); err != nil {
			t.Fatal(err)
		}
		done := decodeResponse[workbench.CopilotTurn](t, request(s, "GET", "/api/copilot/turns/"+turn.ID, "", admin))
		if done.Status != "Succeeded" {
			t.Fatal(done.Status, done.Error)
		}
		return done
	}
	first := runTurn(decodeResponse[workbench.CopilotTurn](t, request(s, "POST", path, `{"intentId":"candidate-http-original","text":"请改写得更含蓄"}`, admin)))
	var artifacts []workbench.CopilotArtifact
	if err := json.Unmarshal(first.Artifacts, &artifacts); err != nil {
		t.Fatal(err)
	}
	var parent workbench.CopilotArtifact
	for _, artifact := range artifacts {
		if artifact.Kind == "revision-candidate" {
			parent = artifact
		}
	}
	if parent.RunID == "" {
		t.Fatal("parent candidate missing")
	}
	body := jsonBody(workbench.CopilotTurnRequest{IntentID: "candidate-http-iteration", Text: "保留候选其他部分，只改这段", CandidateRunID: parent.RunID, CandidateSelection: parent.Text})
	bound := decodeResponse[workbench.CopilotTurn](t, request(s, "POST", path, body, admin))
	if bound.Input.Candidate == nil || bound.Input.Candidate.RunID != parent.RunID || bound.Input.CandidateSelection != parent.Text {
		t.Fatal("HTTP binding lost", bound.Input)
	}
	replay := decodeResponse[workbench.CopilotTurn](t, request(s, "POST", path, body, admin))
	if replay.ID != bound.ID {
		t.Fatal("intent replay created another turn")
	}
	second := runTurn(bound)
	if err := json.Unmarshal(second.Artifacts, &artifacts); err != nil {
		t.Fatal(err)
	}
	var child workbench.CopilotArtifact
	for _, artifact := range artifacts {
		if artifact.Kind == "revision-candidate" {
			child = artifact
		}
	}
	if child.RunID == "" || child.ParentCandidateRunID != parent.RunID || child.SourceRevisionID != r.ID {
		t.Fatal("HTTP result lost lineage", child)
	}
	original := decodeResponse[workbench.Chapter](t, request(s, "GET", "/api/chapters/"+c.ID, "", admin))
	if original.CurrentRevisionID != r.ID {
		t.Fatal("iteration adopted without author")
	}
	other := adminUser(t, s, admin, "candidate-foreign-author")
	decodeResponse[[]workbench.Member](t, request(s, "POST", "/api/projects/"+p.ID+"/members", jsonBody(map[string]string{"username": other.Username, "role": "editor"}), admin))
	otherLogin := loginNamed(t, s, other.Username, "strong-password-12")
	otherSession := decodeResponse[workbench.CopilotSession](t, request(s, "POST", "/api/projects/"+p.ID+"/copilot/sessions", jsonBody(workbench.CopilotSessionInput{ChapterID: c.ID}), otherLogin))
	if w := request(s, "POST", "/api/copilot/sessions/"+otherSession.ID+"/turns", jsonBody(workbench.CopilotTurnRequest{IntentID: "candidate-http-private-owner", Text: "不要继承其他作者的私有上下文", CandidateRunID: child.RunID}), otherLogin); w.Code != 403 {
		t.Fatal("cross-author continuation accepted", w.Code, w.Body.String())
	}
	saved := decodeResponse[workbench.Revision](t, request(s, "POST", "/api/chapters/"+c.ID+"/revisions", jsonBody(map[string]any{"expectedCurrentRevisionId": r.ID, "document": workbench.DocumentFromText(child.Text), "source": "ai", "runId": child.RunID}), admin))
	if saved.ParentID != r.ID {
		t.Fatal("iteration was not adopted against saved original", saved)
	}
	if w := request(s, "POST", path, jsonBody(workbench.CopilotTurnRequest{IntentID: "candidate-http-after-adoption", Text: "旧原稿上的父候选已经失效", CandidateRunID: parent.RunID}), admin); w.Code != 409 {
		t.Fatal("old original still bound", w.Code, w.Body.String())
	}
}
