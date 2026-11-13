package workbench

import (
	"context"
	"encoding/json"
	"novel/internal/llm"
	"strings"
	"testing"
)

func featureProvider(s *Service) {
	s.ProviderName = "fake"
	s.Model = "fake-v1"
	s.MaxOutputTokens = 1500
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Text: "作者确认前的场景、文风或编辑候选。", Raw: []byte("候选")}, nil
	}}
}
func featureFinish(t *testing.T, s *Service, run Run) Run {
	t.Helper()
	claimed, ok, e := s.ClaimRun(context.Background(), "feature-test")
	if e != nil || !ok {
		t.Fatal("claim", ok, e)
	}
	if e = s.ExecuteRun(context.Background(), claimed); e != nil {
		t.Fatal(e)
	}
	got, e := s.GetRun(context.Background(), run.ID)
	if e != nil || got.Status != "Succeeded" || got.Stale {
		t.Fatalf("run not successful %#v %v", got, e)
	}
	return got
}
func TestStudioCanvasSelectionSceneCandidateAndBoardStale(t *testing.T) {
	s := newCore(t)
	featureProvider(s)
	p, c := setupCore(t, s)
	ctx := context.Background()
	board, e := s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{Data: StudioBoard{Nodes: []StudioNode{{ID: "n1", Type: "plot", Label: "收到铜印", X: 10, Y: 20}, {ID: "n2", Type: "scene", Label: "雨夜追索", X: 30, Y: 40, PrerequisiteIDs: []string{"n1"}}}, Edges: []StudioEdge{{ID: "e1", Source: "n1", Target: "n2", Kind: "cause"}}}})
	if e != nil {
		t.Fatal(e)
	}
	run, e := s.SubmitCanvasTask(ctx, p.ID, CanvasTaskInput{ChapterID: c.ID, NodeIDs: []string{"n1", "n2"}, IntentID: "node-scene"}, "canvas-scene")
	if e != nil {
		t.Fatal(e)
	}
	promptRaw, _ := json.Marshal(run.Input.Request)
	if len(run.Input.CanvasNodeIDs) != 2 || run.Input.CanvasBoardVersion != 1 || !strings.Contains(string(promptRaw), "收到铜印") {
		raw, _ := json.Marshal(run.Input.Request)
		t.Fatal("canvas not in actual prompt", string(raw))
	}
	run = featureFinish(t, s, run)
	scene := StudioScene{ChapterID: c.ID, Order: 1, Title: "雨夜追索", Goal: run.Output.Text}
	saved, e := s.ApplyCanvasScene(ctx, p.ID, ApplySceneInput{RunID: run.ID, Scene: scene})
	if e != nil || saved.Version != 1 || len(saved.Data.Scenes) != 1 || len(saved.Data.Scenes[0].NodeIDs) != 2 {
		t.Fatalf("apply %v %#v", e, saved)
	}
	chapter, e := s.GetChapter(ctx, c.ID)
	if e != nil || chapter.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("scene adoption changed prose")
	}
	next, e := s.SubmitCanvasTask(ctx, p.ID, CanvasTaskInput{ChapterID: c.ID, NodeIDs: []string{"n2"}, IntentID: "stale-scene"}, "canvas-scene")
	if e != nil {
		t.Fatal(e)
	}
	next = featureFinish(t, s, next)
	board.Data.Nodes[1].Content = "新的场景理解"
	if _, e = s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{ExpectedVersion: board.Version, Data: board.Data}); e != nil {
		t.Fatal(e)
	}
	scene.Order = 2
	if _, e = s.ApplyCanvasScene(ctx, p.ID, ApplySceneInput{ExpectedVersion: 1, RunID: next.ID, Scene: scene}); !IsConflict(e) {
		t.Fatal("old board candidate adopted", e)
	}
	foreign, other := setupCore(t, s)
	if _, e = s.SubmitCanvasTask(ctx, foreign.ID, CanvasTaskInput{ChapterID: other.ID, NodeIDs: []string{"n1"}, IntentID: "foreign-node"}, "canvas-scene"); e == nil {
		t.Fatal("foreign node accepted")
	}
}
func TestStudioVoiceExplicitAdoptionAndHookReportOnly(t *testing.T) {
	s := newCore(t)
	featureProvider(s)
	p, c := setupCore(t, s)
	ctx := context.Background()
	run, e := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: "voice", TaskType: "voice"})
	if e != nil {
		t.Fatal(e)
	}
	run = featureFinish(t, s, run)
	detail, e := s.GetProject(ctx, p.ID)
	if e != nil || len(detail.Settings) != 0 {
		t.Fatal("voice wrote style before consent")
	}
	setting, e := s.ApplyVoiceProfile(ctx, p.ID, ApplyVoiceInput{RunID: run.ID, Name: "作者文风", Content: "短句、具体动作，克制解释。"})
	if e != nil || setting.Kind != "style" {
		t.Fatal("voice apply", e)
	}
	if _, e = s.ApplyVoiceProfile(ctx, p.ID, ApplyVoiceInput{RunID: run.ID, Name: "过期文风", Content: "不应再采用"}); !IsConflict(e) {
		t.Fatal("stale voice adopted", e)
	}
	hook, e := s.SubmitHookReview(ctx, c.ID, RunRequest{IntentID: "hook"})
	if e != nil || hook.Type != "developmental" || hook.Input.Feature != "hook" || !strings.Contains(hook.Input.Instructions, "章末") {
		t.Fatal("hook specialized prompt missing", e)
	}
	hook = featureFinish(t, s, hook)
	if _, e = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(hook.Output.Text), Source: "ai", RunID: hook.ID}); !IsConflict(e) {
		t.Fatal("hook report adopted as prose", e)
	}
	chapter, e := s.GetChapter(ctx, c.ID)
	if e != nil || chapter.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("hook changed authoritative prose")
	}
}
