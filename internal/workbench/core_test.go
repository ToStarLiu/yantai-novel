package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"novel/internal/job"
	"novel/internal/testutil"
)

func newCore(t *testing.T) *Service { t.Helper(); return &Service{DB: testutil.Database(t)} }
func setupCore(t *testing.T, s *Service) (Project, Chapter) {
	t.Helper()
	ctx := context.Background()
	p, err := s.CreateProject(ctx, "断桥测试", "原创测试", "悬疑")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateChapter(ctx, p.ID, "第一章", "第一卷")
	if err != nil {
		t.Fatal(err)
	}
	return p, c
}

func TestAICandidateRequiresMatchingCurrentSourceAndProject(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	_, other := setupCore(t, s)
	ctx := context.Background()
	runID := job.NewID()
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(ctx, "INSERT INTO wb_runs(id,project_id,chapter_id,type,status,input,intent_id,input_hash,epoch,revision_id) VALUES($1,$2,$3,'draft','Succeeded','{\"blueprintVersion\":1,\"stateVersionId\":\"\"}',$4,'test',$6,$5)", runID, p.ID, c.ID, job.NewID(), c.CurrentRevisionID, detail.Project.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRevision(ctx, other.ID, SaveInput{ExpectedCurrentRevisionID: other.CurrentRevisionID, Document: DocumentFromText("foreign candidate"), Source: "ai", RunID: runID}); !IsConflict(err) {
		t.Fatalf("foreign candidate accepted: %v", err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("missing candidate"), Source: "ai", RunID: "unknown"}); !IsConflict(err) {
		t.Fatalf("unknown candidate accepted: %v", err)
	}
	accepted, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("作者采用的部分修订"), Source: "ai", RunID: runID})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.RunID != runID {
		t.Fatal("AI provenance missing")
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: accepted.ID, Document: DocumentFromText("stale candidate"), Source: "ai", RunID: runID}); !IsConflict(err) {
		t.Fatalf("stale candidate accepted: %v", err)
	}
	for _, kind := range []string{"review", "memory-extraction", "planning", "handoff"} {
		if _, err = s.DB.Exec(ctx, "UPDATE wb_runs SET revision_id=$2,type=$3 WHERE id=$1", runID, accepted.ID, kind); err != nil {
			t.Fatal(err)
		}
		if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: accepted.ID, Document: DocumentFromText("invalid task type"), Source: "ai", RunID: runID}); !IsConflict(err) {
			t.Fatalf("task type %s allowed as draft", kind)
		}
	}
	if _, err = s.DB.Exec(ctx, "UPDATE wb_runs SET revision_id=$2,type='draft' WHERE id=$1", runID, accepted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateChapter(ctx, c.ID, c.Title, c.Volume, "新的章纲"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: accepted.ID, Document: DocumentFromText("blueprint stale"), Source: "ai", RunID: runID}); !IsConflict(err) {
		t.Fatal("outdated blueprint candidate accepted")
	}
}
func saveCore(t *testing.T, s *Service, c Chapter, text string) Revision {
	t.Helper()
	r, err := s.SaveRevision(context.Background(), c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(text)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDocumentPlainTextDerivedAndUnsafeNodesRejected(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"标题"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"林川左臂受伤。"}]}]}]},{"type":"paragraph","content":[{"type":"text","text":"铜印"},{"type":"hardBreak"},{"type":"text","text":"仍在。"}]}]}`)
	_, text, hash, err := NormalizeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if text != "标题\n林川左臂受伤。\n铜印\n仍在。" || len(hash) != 64 {
		t.Fatalf("derived %q hash %s", text, hash)
	}
	for _, bad := range []string{`{"type":"script","text":"bad"}`, `{"type":"doc","content":[{"type":"image","attrs":{"src":"remote"}}]}`, `{"type":"doc","content":[]} garbage`} {
		if _, _, _, err = NormalizeDocument([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestSaveRevisionConcurrentConflictAndHistoryImmutable(t *testing.T) {
	s := newCore(t)
	_, c := setupCore(t, s)
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	success, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("中文段落保存")})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else if IsConflict(err) {
				conflicts++
			} else {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
	old, err := s.GetRevision(ctx, c.CurrentRevisionID)
	if err != nil || old.PlainText != "" {
		t.Fatalf("old revision changed: %#v %v", old, err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE wb_revisions SET plain_text='overwrite' WHERE id=$1", old.ID); err == nil {
		t.Fatal("database allowed revision overwrite")
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := s.RestoreRevision(ctx, c.ID, old.ID, current.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID == old.ID || restored.ParentID != current.CurrentRevisionID || restored.Source != "restored" || restored.PlainText != "" {
		t.Fatalf("restore did not create new history: %#v", restored)
	}
}

func TestCrossProjectReferencesRejected(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	_, other := setupCore(t, s)
	ctx := context.Background()
	if _, err := s.Finalize(ctx, c.ID, other.CurrentRevisionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign finalize: %v", err)
	}
	if _, err := s.RestoreRevision(ctx, c.ID, other.CurrentRevisionID, c.CurrentRevisionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign restore: %v", err)
	}
	setting, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "character", Name: "林川"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveSetting(ctx, other.ProjectID, setting); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign setting: %v", err)
	}
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReorderChapters(ctx, p.ID, detail.Project.Epoch, []string{other.ID}); err == nil {
		t.Fatal("foreign reorder accepted")
	}
}

func TestFinalizeReorderDeleteAtomicImpact(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	r := saveCore(t, s, c, "林川左臂受伤。")
	if _, err := s.Finalize(ctx, c.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Project.Health != "MemoryPending" {
		t.Fatalf("health=%s", detail.Project.Health)
	}
	second, err := s.CreateChapter(ctx, p.ID, "第二章", "")
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	newRevision := saveCore(t, s, current, "林川右臂受伤。")
	_, err = s.DB.Exec(ctx, `CREATE FUNCTION refuse_impact() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$; CREATE TRIGGER fail_impact BEFORE INSERT ON wb_impacts FOR EACH ROW EXECUTE FUNCTION refuse_impact();`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, c.ID, newRevision.ID); err == nil {
		t.Fatal("injected impact failure ignored")
	}
	current, err = s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.FinalRevisionID != r.ID {
		t.Fatal("partial finalize survived rollback")
	}
	_, err = s.DB.Exec(ctx, "DROP TRIGGER fail_impact ON wb_impacts; DROP FUNCTION refuse_impact();")
	if err != nil {
		t.Fatal(err)
	}
	impact, err := s.Finalize(ctx, c.ID, newRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.ChapterIDs) != 2 || impact.EarliestOrder != 1 {
		t.Fatalf("impact %#v", impact)
	}
	second, err = s.GetChapter(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !second.NeedsReview {
		t.Fatal("downstream review not marked")
	}
	detail, err = s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Project.Health != "RebuildRequired" {
		t.Fatal("rebuild health missing")
	}
	if _, err = s.ReorderChapters(ctx, p.ID, detail.Project.Epoch-1, []string{second.ID, c.ID}); !IsConflict(err) {
		t.Fatalf("stale order: %v", err)
	}
	if _, err = s.ReorderChapters(ctx, p.ID, detail.Project.Epoch, []string{second.ID, c.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteChapter(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetChapter(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted visible: %v", err)
	}
	if rev, err := s.GetRevision(ctx, r.ID); err != nil || !strings.Contains(rev.PlainText, "左臂") {
		t.Fatal("deleted chapter lost source history")
	}
}
