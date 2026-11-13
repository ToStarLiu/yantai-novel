package server

import (
	"context"
	"novel/internal/workbench"
	"testing"
)

func TestStudioHTTPPersistentCanvasBriefReferenceBranchAndSandbox(t *testing.T) {
	s := testServer(t)
	session := loginForTest(t, s)
	project := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"高级工作室验证"}`, session))
	chapter := decodeResponse[workbench.Chapter](t, request(s, "POST", "/api/projects/"+project.ID+"/chapters", `{"title":"第一章"}`, session))
	actor := decodeResponse[workbench.Setting](t, request(s, "POST", "/api/projects/"+project.ID+"/settings", `{"kind":"character","name":"林川"}`, session))
	base := "/api/projects/" + project.ID + "/studio"
	if w := request(s, "GET", base, "", testLogin{}); w.Code != 401 {
		t.Fatalf("anonymous studio status %d", w.Code)
	}
	view := decodeResponse[workbench.StudioView](t, request(s, "GET", base, "", session))
	if view.Board.Version != 0 {
		t.Fatal("new board unexpectedly exists")
	}
	board := workbench.StudioBoard{Nodes: []workbench.StudioNode{{ID: "n1", Type: "scene", Label: "夜雨入城", X: 20, Y: 40, ChapterID: chapter.ID, ObservedBy: []string{actor.ID}}, {ID: "n2", Type: "plot", Label: "发现真相", X: 300, Y: 40, PrerequisiteIDs: []string{"n1"}}}}
	saved := decodeResponse[workbench.StudioDocument[workbench.StudioBoard]](t, request(s, "PUT", base+"/board", jsonBody(workbench.StudioWrite[workbench.StudioBoard]{Data: board}), session))
	if saved.Version != 1 {
		t.Fatal("board not persisted")
	}
	if w := request(s, "PUT", base+"/board", jsonBody(workbench.StudioWrite[workbench.StudioBoard]{Data: board}), session); w.Code != 409 {
		t.Fatalf("stale board not409 %d", w.Code)
	}
	evaluation := decodeResponse[workbench.BoardEvaluation](t, request(s, "POST", base+"/board/evaluate", `{"completedNodeIds":[]}`, session))
	if len(evaluation.AvailableNodeIDs) != 1 {
		t.Fatal("board evaluation wrong")
	}
	brief := workbench.StoryBrief{Audience: "悬疑读者", CoreConflict: "信任与谎言", FailureCost: "失去同伴", TargetWords: 50000, TargetChapters: 20, Volumes: []workbench.StoryVolume{{ID: "v1", Title: "追索", ChapterCount: 10}}}
	_ = decodeResponse[workbench.StudioDocument[workbench.StoryBrief]](t, request(s, "PUT", base+"/brief", jsonBody(workbench.StudioWrite[workbench.StoryBrief]{Data: brief}), session))
	ref := decodeResponse[workbench.StudioReference](t, request(s, "POST", base+"/references", `{"title":"结构参考","text":"# 一\n冰封狼印记录在资料中。","sourceNote":"原创测试"}`, session))
	if ref.Analysis.ParagraphCount != 2 {
		t.Fatal("reference structure missing")
	}
	hits := decodeResponse[[]workbench.ReferenceHit](t, request(s, "GET", base+"/references/search?q=冰封狼印", "", session))
	if len(hits) != 1 {
		t.Fatal("isolated reference search missing")
	}
	noCanonical := decodeResponse[[]workbench.Citation](t, request(s, "GET", "/api/projects/"+project.ID+"/search?q=冰封狼印", "", session))
	if len(noCanonical) != 0 {
		t.Fatal("reference entered canonical index")
	}
	box := decodeResponse[workbench.Sandbox](t, request(s, "POST", base+"/sandboxes", jsonBody(workbench.SandboxCreate{Title: "独立局势", EntityIDs: []string{actor.ID}, Steps: 2, Parameters: workbench.SandboxParameters{InitialResources: 20, InitialTension: 30, ResourceDelta: 1, TensionDelta: 1}}), session))
	if box.Candidate == nil || box.State.Turn != 0 {
		t.Fatal("simulation prematurely committed")
	}
	box = decodeResponse[workbench.Sandbox](t, request(s, "POST", base+"/sandboxes/"+box.ID+"/confirm", jsonBody(map[string]any{"expectedVersion": box.Version}), session))
	if box.State.Turn != 2 {
		t.Fatal("sandbox confirmation missing")
	}
	detail := decodeResponse[workbench.ProjectDetail](t, request(s, "GET", "/api/projects/"+project.ID, "", session))
	fork := decodeResponse[workbench.ProjectDetail](t, request(s, "POST", base+"/fork", jsonBody(map[string]any{"title": "另一条线", "expectedEpoch": detail.Project.Epoch}), session))
	if fork.Project.ID == project.ID {
		t.Fatal("fork reused source")
	}
	copied := decodeResponse[workbench.StudioView](t, request(s, "GET", "/api/projects/"+fork.Project.ID+"/studio", "", session))
	if len(copied.Board.Data.Nodes) != 2 || copied.Board.Data.Nodes[0].ID == "n1" || copied.Brief.Data.CoreConflict != brief.CoreConflict || copied.Sandboxes[0].State.Turn != 2 {
		t.Fatal("studio archive not remapped/preserved")
	}
	branches := decodeResponse[[]workbench.BranchLink](t, request(s, "GET", base+"/branches", "", session))
	if len(branches) != 1 || branches[0].TargetProjectID != fork.Project.ID {
		t.Fatal("branch registry missing")
	}
	if err := s.Service.DB.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
