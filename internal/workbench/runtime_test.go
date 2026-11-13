package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"novel/internal/job"
	"novel/internal/llm"
	"novel/internal/testutil"
	"sync/atomic"
	"testing"
	"time"
)

func runtimeFixture(t *testing.T) (*Service, Chapter) {
	t.Helper()
	s := &Service{DB: testutil.Database(t), Model: "fake-v1", ProviderName: "fake", MaxOutputTokens: 500, MaxConcurrent: 1, JobTimeout: time.Second}
	ctx := context.Background()
	p, err := s.CreateProject(ctx, "运行测试", "", "悬疑")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateChapter(ctx, p.ID, "第一章", "")
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.UpdateChapter(ctx, c.ID, c.Title, "", "保持人物知识边界，续写下一段")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("林川左臂受伤。"), Source: "author"})
	if err != nil {
		t.Fatal(err)
	}
	c.CurrentRevisionID = r.ID
	return s, c
}
func submit(t *testing.T, s *Service, c Chapter, kind string) Run {
	t.Helper()
	r, e := s.SubmitRun(context.Background(), c.ID, RunRequest{IntentID: job.NewID(), TaskType: kind, InputRevisionID: c.CurrentRevisionID})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func claim(t *testing.T, s *Service) Run {
	t.Helper()
	r, ok, e := s.ClaimRun(context.Background(), job.NewID())
	if e != nil || !ok {
		t.Fatalf("claim %v %v", ok, e)
	}
	return r
}

func TestRuntimeDedupClaimAndPersistentCompletion(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	s.Provider = &llm.Fake{Handler: func(ctx context.Context, r llm.Request) (llm.Response, error) {
		calls.Add(1)
		var count int
		if e := s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_run_attempts WHERE status='Running'").Scan(&count); e != nil || count != 1 {
			t.Fatalf("attempt not committed: %d %v", count, e)
		}
		return llm.Response{Text: "候选保持左臂受伤。", Raw: []byte(`{"output":"candidate"}`)}, nil
	}}
	first := submit(t, s, c, "draft")
	again, e := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: first.IntentID, TaskType: "draft", InputRevisionID: c.CurrentRevisionID})
	if e != nil || again.ID != first.ID {
		t.Fatal("intent duplicated", e)
	}
	r := claim(t, s)
	if _, ok, e := s.ClaimRun(ctx, "other"); e != nil || ok {
		t.Fatal("global concurrency exceeded", e)
	}
	if e = s.ExecuteRun(ctx, r); e != nil {
		t.Fatal(e)
	}
	done, e := s.GetRun(ctx, r.ID)
	if e != nil || done.Status != "Succeeded" || done.Output.Text == "" || calls.Load() != 1 {
		t.Fatalf("completion %+v %v", done, e)
	}
	fresh, e := s.GetChapter(ctx, c.ID)
	if e != nil || fresh.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("AI overwrote manuscript", e)
	}
	events, e := s.Events(ctx, r.ID, 0)
	if e != nil || len(events) < 4 {
		t.Fatal("events not durable", e)
	}
	next, e := s.Events(ctx, r.ID, events[0].ID)
	if e != nil || len(next) != len(events)-1 {
		t.Fatal("SSE replay wrong", e)
	}
}
func TestRuntimeUnknownNeverRetriesAndLeaseFencing(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	var calls int
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		return llm.Response{}, errors.New("lost after send")
	}}
	submit(t, s, c, "draft")
	r := claim(t, s)
	if e := s.ExecuteRun(ctx, r); e != nil {
		t.Fatal(e)
	}
	done, _ := s.GetRun(ctx, r.ID)
	if done.Status != "NeedsReconciliation" {
		t.Fatal(done.Status)
	}
	if _, e := s.RunAction(ctx, r.ID, "retry"); e == nil {
		t.Fatal("unknown retried")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	second := submit(t, s, c, "draft")
	old := claim(t, s)
	_, e := s.DB.Exec(ctx, "UPDATE wb_runs SET lease_until=now()-interval '1 second' WHERE id=$1", second.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RecoverExpired(ctx); e != nil {
		t.Fatal(e)
	}
	newer := claim(t, s)
	if old.LeaseGeneration >= newer.LeaseGeneration {
		t.Fatal("lease generation not advanced")
	}
	if e = s.ExecuteRun(ctx, old); e == nil {
		t.Fatal("old worker submitted")
	}
	if calls != 1 {
		t.Fatal("old worker dispatched")
	}
}
func TestRuntimeCancellationPauseAndStaleCandidate(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	first := submit(t, s, c, "draft")
	r := claim(t, s)
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		if _, e := s.RunAction(ctx, first.ID, "cancel"); e != nil {
			t.Fatal(e)
		}
		return llm.Response{Text: "取消前已生成", Raw: []byte("partial")}, nil
	}}
	if e := s.ExecuteRun(ctx, r); e != nil {
		t.Fatal(e)
	}
	done, _ := s.GetRun(ctx, r.ID)
	if done.Status != "Cancelled" || done.Output.Text == "" {
		t.Fatalf("cancel lost output: %+v", done)
	}
	paused := submit(t, s, c, "draft")
	if _, e := s.RunAction(ctx, paused.ID, "pause"); e != nil {
		t.Fatal(e)
	}
	if _, ok, _ := s.ClaimRun(ctx, "a"); ok {
		t.Fatal("paused dispatched")
	}
	if _, e := s.RunAction(ctx, paused.ID, "resume"); e != nil {
		t.Fatal(e)
	}
	r = claim(t, s)
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		_, e := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("作者已手动改稿。"), Source: "author"})
		if e != nil {
			t.Fatal(e)
		}
		return llm.Response{Text: "旧输入候选", Raw: []byte("response")}, nil
	}}
	if e := s.ExecuteRun(ctx, r); e != nil {
		t.Fatal(e)
	}
	done, _ = s.GetRun(ctx, r.ID)
	if !done.Stale || done.Status != "Succeeded" {
		t.Fatalf("stale hidden: %+v", done)
	}
}
func TestRuntimeExpiredExternalAttemptIsUnknown(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	submit(t, s, c, "draft")
	r := claim(t, s)
	if _, e := s.beginCall(ctx, r); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, "UPDATE wb_runs SET lease_until=now()-interval '1 second' WHERE id=$1", r.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.RecoverExpired(ctx); e != nil {
		t.Fatal(e)
	}
	done, _ := s.GetRun(ctx, r.ID)
	if done.Status != "NeedsReconciliation" {
		t.Fatal(done.Status)
	}
	var request json.RawMessage
	if e := s.DB.QueryRow(ctx, "SELECT request_snapshot FROM wb_run_attempts WHERE run_id=$1", r.ID).Scan(&request); e != nil || len(request) == 0 {
		t.Fatal("attempt snapshot missing", e)
	}
}
