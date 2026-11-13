package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"novel/internal/llm"
	"novel/internal/testutil"
)

type narrativeFixture struct {
	s                               *Service
	p                               Project
	lin, shen, seal, secret, bridge Setting
}

func newNarrativeFixture(t *testing.T) narrativeFixture {
	t.Helper()
	s := &Service{DB: testutil.Database(t)}
	ctx := context.Background()
	p, err := s.CreateProject(ctx, "断桥铜印", "受控故事", "悬疑")
	if err != nil {
		t.Fatal(err)
	}
	f := narrativeFixture{s: s, p: p}
	specs := []struct {
		dst        *Setting
		kind, name string
	}{{&f.lin, "character", "林川"}, {&f.shen, "character", "沈月"}, {&f.seal, "world", "铜印"}, {&f.secret, "world", "铜印来历"}, {&f.bridge, "world", "桥孔凿痕"}}
	for _, spec := range specs {
		value, e := s.SaveSetting(ctx, p.ID, Setting{Kind: spec.kind, Name: spec.name})
		if e != nil {
			t.Fatal(e)
		}
		*spec.dst = value
	}
	return f
}
func (f narrativeFixture) chapter(t *testing.T, title, text string) Chapter {
	t.Helper()
	ctx := context.Background()
	c, err := f.s.CreateChapter(ctx, f.p.ID, title, "第一卷")
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(text)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Finalize(ctx, c.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	c, err = f.s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func (f narrativeFixture) change(t *testing.T, c Chapter, kind, subject, predicate, object, character, truth, quote string) MemoryChange {
	t.Helper()
	r, err := f.s.GetRevision(context.Background(), c.FinalRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	return MemoryChange{Kind: kind, Subject: subject, Predicate: predicate, Object: object, CharacterID: character, TruthStatus: truth, Evidence: Evidence{RevisionID: r.ID, PlainTextHash: r.Hash, Quote: quote}}
}
func (f narrativeFixture) proposal(t *testing.T, c Chapter, changes []MemoryChange) Proposal {
	t.Helper()
	mc, err := f.s.MemoryInput(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.s.CreateProposal(context.Background(), c.ID, mc.SourceRevisionID, mc.BaseStateID, mc.Epoch, changes)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f narrativeFixture) apply(t *testing.T, p Proposal) StateView {
	t.Helper()
	decisions := []MemoryDecision{}
	for _, c := range p.Changes {
		decisions = append(decisions, MemoryDecision{ID: c.ID, Decision: "Accept"})
	}
	if _, err := f.s.DecideProposal(context.Background(), p.ID, decisions); err != nil {
		t.Fatal(err)
	}
	v, err := f.s.ApplyProposal(context.Background(), p.ID, p.BaseStateID, p.SourceRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNarrativeFiveKindsRoleBeliefsAndIdempotence(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	text := "林川留在北岸磨坊，左臂仍受伤。铜印一直由林川持有。沈月知道铜印来自王都司钥署，林川却以为是商队凭牌。桥孔的凿痕尚未解释。"
	c := f.chapter(t, "第一章", text)
	changes := []MemoryChange{
		f.change(t, c, "Location", f.lin.ID, "location", "北岸磨坊", "", "True", "林川留在北岸磨坊"),
		f.change(t, c, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂仍受伤"),
		f.change(t, c, "Ownership", f.seal.ID, "owner", f.lin.ID, "", "True", "铜印一直由林川持有"),
		f.change(t, c, "Knowledge", f.secret.ID, "origin", "王都司钥署", f.shen.ID, "True", "沈月知道铜印来自王都司钥署"),
		f.change(t, c, "Knowledge", f.secret.ID, "origin", "商队凭牌", f.lin.ID, "False", "林川却以为是商队凭牌"),
		f.change(t, c, "Foreshadow", f.bridge.ID, "status", "未解释", "", "True", "桥孔的凿痕尚未解释"),
	}
	p := f.proposal(t, c, changes)
	for _, c := range p.Changes {
		if c.Decision != "Pending" || c.ID == "" {
			t.Fatal("model output must require a new author decision")
		}
	}
	if _, err := f.s.ApplyProposal(ctx, p.ID, p.BaseStateID, p.SourceRevisionID); err == nil {
		t.Fatal("unreviewed proposal applied")
	}
	state := f.apply(t, p)
	if len(state.Facts) != 6 || state.Health != "Ready" {
		t.Fatalf("state %#v", state)
	}
	falseBelief := false
	truth := false
	for _, fact := range state.Facts {
		if fact.StoryTime != nil {
			t.Fatal("unknown time became day 0")
		}
		if fact.CharacterID == f.lin.ID && fact.TruthStatus == "False" {
			falseBelief = true
		}
		if fact.CharacterID == f.shen.ID && fact.Object == "王都司钥署" {
			truth = true
		}
	}
	if !falseBelief || !truth {
		t.Fatal("objective truth and character beliefs were collapsed")
	}
	if _, err := f.s.ApplyProposal(ctx, p.ID, p.BaseStateID, p.SourceRevisionID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_facts WHERE project_id=$1", f.p.ID).Scan(&count); err != nil || count != 6 {
		t.Fatalf("replay duplicated facts %d %v", count, err)
	}
}

func TestNarrativeRejectsInventedEvidenceForeignEntitiesAndStaleVersions(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "第一章", "林川左臂受伤。")
	original := f.change(t, c, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂受伤")
	mc, _ := f.s.MemoryInput(ctx, c.ID)
	bad := original
	bad.Evidence.Quote = "铜印已熔毁"
	if _, err := f.s.CreateProposal(ctx, c.ID, mc.SourceRevisionID, mc.BaseStateID, mc.Epoch, []MemoryChange{bad}); err == nil {
		t.Fatal("invented evidence accepted")
	}
	other, err := f.s.CreateProject(ctx, "别的作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.s.SaveSetting(ctx, other.ID, Setting{Kind: "character", Name: "外人"})
	if err != nil {
		t.Fatal(err)
	}
	bad = original
	bad.Subject = foreign.ID
	if _, err := f.s.CreateProposal(ctx, c.ID, mc.SourceRevisionID, mc.BaseStateID, mc.Epoch, []MemoryChange{bad}); err == nil {
		t.Fatal("foreign entity accepted")
	}
	bad = original
	bad.Subject = "invented-stable-id"
	if _, err := f.s.CreateProposal(ctx, c.ID, mc.SourceRevisionID, mc.BaseStateID, mc.Epoch, []MemoryChange{bad}); err == nil {
		t.Fatal("unknown stable ID accepted")
	}
	p := f.proposal(t, c, []MemoryChange{original})
	edited, err := f.s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("林川右臂受伤。")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Finalize(ctx, c.ID, edited.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DecideProposal(ctx, p.ID, []MemoryDecision{{ID: p.Changes[0].ID, Decision: "Accept"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ApplyProposal(ctx, p.ID, p.BaseStateID, p.SourceRevisionID); !IsConflict(err) {
		t.Fatalf("stale proposal accepted: %v", err)
	}
}

func TestNarrativeCompetingBaselinesAndRejectedChanges(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "第一章", "林川左臂受伤。")
	change := f.change(t, c, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂受伤")
	p := f.proposal(t, c, []MemoryChange{change})
	competing := f.proposal(t, c, []MemoryChange{change})
	if _, err := f.s.DecideProposal(ctx, p.ID, []MemoryDecision{{ID: p.Changes[0].ID, Decision: "Reject"}}); err != nil {
		t.Fatal(err)
	}
	state, err := f.s.ApplyProposal(ctx, p.ID, p.BaseStateID, p.SourceRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Facts) != 0 {
		t.Fatal("rejected proposal created a fact")
	}
	if _, err = f.s.DecideProposal(ctx, competing.ID, []MemoryDecision{{ID: competing.Changes[0].ID, Decision: "Accept"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ApplyProposal(ctx, competing.ID, competing.BaseStateID, competing.SourceRevisionID); !IsConflict(err) {
		t.Fatalf("competing baseline overwrite: %v", err)
	}
}

func TestNarrativeTwentyChaptersRebuildAndReorder(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapters := []Chapter{}
	for i := 1; i <= 20; i++ {
		c := f.chapter(t, fmt.Sprintf("第%d章", i), fmt.Sprintf("林川到达第%d座驿站，左臂仍受伤。", i))
		chapters = append(chapters, c)
		f.apply(t, f.proposal(t, c, []MemoryChange{f.change(t, c, "Location", f.lin.ID, "location", fmt.Sprintf("驿站%d", i), "", "True", fmt.Sprintf("林川到达第%d座驿站", i))}))
	}
	state, err := f.s.State(ctx, f.p.ID)
	if err != nil || len(state.Facts) != 1 || state.Facts[0].Object != "驿站20" {
		t.Fatalf("latest projection %v %#v", err, state)
	}
	r, err := f.s.SaveRevision(ctx, chapters[4].ID, SaveInput{ExpectedCurrentRevisionID: chapters[4].CurrentRevisionID, Document: DocumentFromText("林川到达新的第五驿站，铜印被毁。")})
	if err != nil {
		t.Fatal(err)
	}
	impact, err := f.s.Finalize(ctx, chapters[4].ID, r.ID)
	if err != nil || len(impact.ChapterIDs) != 16 {
		t.Fatalf("impact %#v %v", impact, err)
	}
	state, err = f.s.State(ctx, f.p.ID)
	if err != nil || len(state.PendingChapterIDs) != 16 || state.Facts[0].Object != "驿站4" {
		t.Fatalf("rollback checkpoint %#v %v", state, err)
	}
	next, err := f.s.MemoryInput(ctx, chapters[5].ID)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := f.s.CreateProposal(ctx, chapters[5].ID, next.SourceRevisionID, next.BaseStateID, next.Epoch, []MemoryChange{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ApplyProposal(ctx, proposal.ID, proposal.BaseStateID, proposal.SourceRevisionID); !IsConflict(err) {
		t.Fatalf("skipped rebuild chapter: %v", err)
	}
	for i := 4; i < 20; i++ {
		c, err := f.s.GetChapter(ctx, chapters[i].ID)
		if err != nil {
			t.Fatal(err)
		}
		quote := fmt.Sprintf("林川到达第%d座驿站", i+1)
		object := fmt.Sprintf("驿站%d", i+1)
		if i == 4 {
			quote = "林川到达新的第五驿站"
			object = "新驿站5"
		}
		f.apply(t, f.proposal(t, c, []MemoryChange{f.change(t, c, "Location", f.lin.ID, "location", object, "", "True", quote)}))
	}
	state, err = f.s.State(ctx, f.p.ID)
	if err != nil || state.Health != "Ready" || len(state.PendingChapterIDs) != 0 {
		t.Fatalf("rebuild not ready %#v %v", state, err)
	}
	ids := []string{}
	for _, c := range chapters {
		ids = append(ids, c.ID)
	}
	ids[9], ids[10] = ids[10], ids[9]
	impact, err = f.s.ReorderChapters(ctx, f.p.ID, state.Epoch, ids)
	if err != nil || impact.EarliestOrder != 10 {
		t.Fatalf("reorder %#v %v", impact, err)
	}
	state, _ = f.s.State(ctx, f.p.ID)
	if len(state.PendingChapterIDs) != 11 {
		t.Fatalf("wrong reorder pending %d", len(state.PendingChapterIDs))
	}
	text, err := f.s.ConfirmedContext(ctx, f.p.ID, chapters[2].ID, "")
	if err != nil || strings.Contains(text, "驿站3") || !strings.Contains(text, "驿站2") {
		t.Fatalf("future state leaked %q %v", text, err)
	}
}

func TestMemoryDecodeStrictShape(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"changes":null}`, `{"changes":[],"extra":true}`, `{"changes":[]} {}`} {
		if _, err := DecodeMemoryChanges([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := DecodeMemoryChanges([]byte(`{"changes":[]}`)); err != nil {
		t.Fatal(err)
	}
	var schema any
	if err := json.Unmarshal([]byte(MemorySchema()), &schema); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryWorkerCompletionAndProposalAreAtomic(t *testing.T) {
	for _, inject := range []bool{false, true} {
		t.Run(fmt.Sprint("insert-failure-", inject), func(t *testing.T) {
			f := newNarrativeFixture(t)
			ctx := context.Background()
			c := f.chapter(t, "第一章", "林川左臂受伤。")
			change := f.change(t, c, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂受伤")
			data, _ := json.Marshal(map[string]any{"changes": []MemoryChange{change}})
			f.s.Model = "fake-v1"
			f.s.ProviderName = "fake"
			f.s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
				return llm.Response{Text: string(data), Raw: data}, nil
			}}
			run, err := f.s.SubmitRun(ctx, c.ID, RunRequest{IntentID: "memory-atomic", TaskType: "memory-extraction"})
			if err != nil {
				t.Fatal(err)
			}
			claimed, ok, err := f.s.ClaimRun(ctx, "memory-worker")
			if err != nil || !ok || claimed.ID != run.ID {
				t.Fatalf("claim %v %v", ok, err)
			}
			if inject {
				if _, err = f.s.DB.Exec(ctx, `CREATE FUNCTION fail_proposal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected proposal failure'; END $$; CREATE TRIGGER fail_proposal BEFORE INSERT ON wb_proposals FOR EACH ROW EXECUTE FUNCTION fail_proposal()`); err != nil {
					t.Fatal(err)
				}
			}
			executeErr := f.s.ExecuteRun(ctx, claimed)
			saved, err := f.s.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err = f.s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_proposals WHERE project_id=$1", f.p.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if inject {
				if executeErr == nil || saved.Status == "Succeeded" || count != 0 {
					t.Fatalf("partial memory completion escaped: %s count=%d err=%v", saved.Status, count, executeErr)
				}
				if _, err = f.s.DB.Exec(ctx, "UPDATE wb_runs SET lease_until=now()-interval '1 second' WHERE id=$1", run.ID); err != nil {
					t.Fatal(err)
				}
				if err = f.s.RecoverExpired(ctx); err != nil {
					t.Fatal(err)
				}
				saved, err = f.s.GetRun(ctx, run.ID)
				if err != nil || saved.Status != "NeedsReconciliation" {
					t.Fatalf("unknown completed call was replayable: %#v %v", saved, err)
				}
			} else {
				if executeErr != nil || saved.Status != "Succeeded" || count != 1 || saved.Output.ProposalID == "" {
					t.Fatalf("missing atomic proposal: %#v %v", saved, executeErr)
				}
				proposal, err := f.s.GetProposal(ctx, saved.Output.ProposalID)
				if err != nil || proposal.Status != "Pending" {
					t.Fatalf("proposal not reviewable %#v %v", proposal, err)
				}
				events, err := f.s.Events(ctx, run.ID, 0)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, event := range events {
					if event.Kind == "output" && strings.Contains(string(event.Data), saved.Output.ProposalID) {
						found = true
					}
				}
				if !found {
					t.Fatal("durable output event omitted proposal ID")
				}
			}
		})
	}
}

func TestNarrativeRejectsDanglingTemporaryReferencesAndDuplicateUpdates(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "第一章", "旅人周扬到了磨坊，林川把铜印交给周扬。")
	created := f.change(t, c, "Location", "new:zhou-yang", "location", "磨坊", "", "True", "旅人周扬到了磨坊")
	created.EntityName = "周扬"
	transferred := f.change(t, c, "Ownership", f.seal.ID, "owner", "new:zhou-yang", "", "True", "林川把铜印交给周扬")
	p := f.proposal(t, c, []MemoryChange{created, transferred})
	if _, err := f.s.DecideProposal(ctx, p.ID, []MemoryDecision{{ID: p.Changes[0].ID, Decision: "Reject"}, {ID: p.Changes[1].ID, Decision: "Accept"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ApplyProposal(ctx, p.ID, p.BaseStateID, p.SourceRevisionID); err == nil {
		t.Fatal("rejected new entity left dangling ownership")
	}
	if _, err := f.s.DecideProposal(ctx, p.ID, []MemoryDecision{{ID: p.Changes[0].ID, Decision: "Accept"}}); err != nil {
		t.Fatal(err)
	}
	state, err := f.s.ApplyProposal(ctx, p.ID, p.BaseStateID, p.SourceRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range state.Facts {
		if strings.HasPrefix(fact.Subject, "new:") || strings.HasPrefix(fact.Object, "new:") {
			t.Fatal("temporary ID persisted")
		}
	}
	mc, _ := f.s.MemoryInput(ctx, c.ID)
	duplicate := transferred
	duplicate.Object = f.lin.ID
	if _, err = f.s.CreateProposal(ctx, c.ID, mc.SourceRevisionID, mc.BaseStateID, mc.Epoch, []MemoryChange{created, transferred, duplicate}); err == nil {
		t.Fatal("ambiguous duplicate updates accepted")
	}
}

func TestMemoryInputAndGenerationDoNotReadFutureConfirmedFacts(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapters := []Chapter{}
	for i, place := range []string{"北岸磨坊", "东门药铺", "王都密室"} {
		c := f.chapter(t, fmt.Sprintf("第%d章", i+1), "林川到达"+place+"。")
		f.apply(t, f.proposal(t, c, []MemoryChange{f.change(t, c, "Location", f.lin.ID, "location", place, "", "True", "林川到达"+place)}))
		chapters = append(chapters, c)
	}
	input, err := f.s.MemoryInput(ctx, chapters[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input.Facts)
	if strings.Contains(string(raw), "王都密室") || strings.Contains(string(raw), "东门药铺") || !strings.Contains(string(raw), "北岸磨坊") {
		t.Fatalf("memory input future fact leakage %s", raw)
	}
	if _, err = f.s.UpdateChapter(ctx, chapters[1].ID, "第二章", "第一卷", "林川准备药物"); err != nil {
		t.Fatal(err)
	}
	f.s.Model = "fake-v1"
	f.s.ProviderName = "fake"
	run, err := f.s.SubmitRun(ctx, chapters[1].ID, RunRequest{IntentID: "future-knowledge-guard", TaskType: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(run.Input.Memory, "王都密室") || strings.Contains(run.Input.Memory, "东门药铺") || !strings.Contains(run.Input.Memory, "北岸磨坊") {
		t.Fatalf("run memory future leakage %s", run.Input.Memory)
	}
	for _, citation := range run.Input.Citations {
		if citation.ChapterOrder >= chapters[1].Order {
			t.Fatalf("future citation %#v", citation)
		}
	}
}
