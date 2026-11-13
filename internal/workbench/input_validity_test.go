package workbench

import (
	"context"
	"encoding/json"
	"testing"

	"novel/internal/llm"
)

func TestRunRejectsOldBlueprintBeforeCallAndMarksInflightCandidateStale(t *testing.T) {
	for _, during := range []bool{false, true} {
		name := "before-dispatch"
		if during {
			name = "during-call"
		}
		t.Run(name, func(t *testing.T) {
			f := newNarrativeFixture(t)
			ctx := context.Background()
			chapter, err := f.s.CreateChapter(ctx, f.p.ID, "第一章", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.UpdateChapter(ctx, chapter.ID, "第一章", "", "旧章纲"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			f.s.Model = "fake-v1"
			f.s.ProviderName = "fake"
			update := func() {
				if _, err := f.s.UpdateChapter(ctx, chapter.ID, "第一章", "", "新章纲"); err != nil {
					t.Fatal(err)
				}
			}
			f.s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
				calls++
				if during {
					update()
				}
				return llm.Response{Text: "旧章纲候选", Raw: []byte("旧章纲候选")}, nil
			}}
			run, err := f.s.SubmitRun(ctx, chapter.ID, RunRequest{IntentID: "old-blueprint", TaskType: "draft"})
			if err != nil {
				t.Fatal(err)
			}
			claimed, ok, err := f.s.ClaimRun(ctx, "blueprint-worker")
			if err != nil || !ok {
				t.Fatalf("claim %v %v", ok, err)
			}
			if !during {
				update()
			}
			if err = f.s.ExecuteRun(ctx, claimed); err != nil {
				t.Fatal(err)
			}
			saved, err := f.s.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !saved.Stale {
				t.Fatal("old blueprint result is presented as current")
			}
			if during {
				if calls != 1 || saved.Output.Text != "旧章纲候选" {
					t.Fatalf("stale candidate must remain inspectable %#v calls=%d", saved, calls)
				}
			} else if calls != 0 || saved.Status != "Failed" {
				t.Fatalf("old input dispatched %#v calls=%d", saved, calls)
			}
		})
	}
}

func TestMemoryRunCanUseStillFinalizedSourceAfterNewDraftSaved(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter := f.chapter(t, "第一章", "林川左臂受伤。")
	sourceRevision := chapter.FinalRevisionID
	if _, err := f.s.SaveRevision(ctx, chapter.ID, SaveInput{ExpectedCurrentRevisionID: chapter.CurrentRevisionID, Document: DocumentFromText("这是仍未定稿的新编辑稿。")}); err != nil {
		t.Fatal(err)
	}
	change := f.change(t, chapter, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂受伤")
	raw, _ := json.Marshal(map[string]any{"changes": []MemoryChange{change}})
	f.s.Model = "fake-v1"
	f.s.ProviderName = "fake"
	calls := 0
	f.s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		return llm.Response{Text: string(raw), Raw: raw}, nil
	}}
	run, err := f.s.SubmitRun(ctx, chapter.ID, RunRequest{IntentID: "final-authority", TaskType: "memory-extraction"})
	if err != nil {
		t.Fatal(err)
	}
	if run.Input.RevisionID != sourceRevision {
		t.Fatal("memory input followed draft instead of final authority")
	}
	claimed, ok, err := f.s.ClaimRun(ctx, "final-worker")
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	if err = f.s.ExecuteRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	saved, err := f.s.GetRun(ctx, run.ID)
	if err != nil || saved.Stale || saved.Status != "Succeeded" || saved.Output.ProposalID == "" || calls != 1 {
		t.Fatalf("valid final memory was rejected %#v calls=%d err=%v", saved, calls, err)
	}
}

func TestRunResumeRejectsChangedMemoryBaselineWithoutEpochChange(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter := f.chapter(t, "第一章", "林川左臂受伤。")
	proposal := f.proposal(t, chapter, []MemoryChange{f.change(t, chapter, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂受伤")})
	f.s.Model = "fake-v1"
	f.s.ProviderName = "fake"
	run, err := f.s.SubmitRun(ctx, chapter.ID, RunRequest{IntentID: "paused-baseline", TaskType: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RunAction(ctx, run.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	f.apply(t, proposal)
	var epoch int64
	if err = f.s.DB.QueryRow(ctx, "SELECT epoch FROM wb_projects WHERE id=$1", f.p.ID).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if epoch != run.Input.Epoch {
		t.Fatal("test requires a state update without epoch change")
	}
	if _, err = f.s.RunAction(ctx, run.ID, "resume"); !IsConflict(err) {
		t.Fatalf("outdated memory baseline resumed: %v", err)
	}
}
