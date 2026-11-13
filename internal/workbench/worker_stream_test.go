package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"novel/internal/llm"
)

func TestWorkerPersistsRealStreamBeforeCompletionAndReplaysEvents(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter, err := f.s.CreateChapter(ctx, f.p.ID, "第一章", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.UpdateChapter(ctx, chapter.ID, "第一章", "", "雨夜进入磨坊"); err != nil {
		t.Fatal(err)
	}
	flushed := make(chan struct{})
	finish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"林川\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(flushed)
		select {
		case <-finish:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"走进磨坊。\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	provider, err := llm.NewCompatible(llm.Config{BaseURL: server.URL, APIKey: "not-a-real-key", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	f.s.Provider = provider
	f.s.Model = "fake-stream-model"
	f.s.ProviderName = "fake-stream"
	run, err := f.s.SubmitRun(ctx, chapter.ID, RunRequest{IntentID: "persistent-stream", TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := f.s.ClaimRun(ctx, "stream-worker")
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	completed := make(chan error, 1)
	go func() { completed <- f.s.ExecuteRun(ctx, claimed) }()
	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("provider stream never started")
	}
	deadline := time.Now().Add(2 * time.Second)
	var partial Run
	for time.Now().Before(deadline) {
		partial, err = f.s.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if partial.Output.Text == "林川" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if partial.Output.Text != "林川" || partial.Status != "Running" {
		close(finish)
		t.Fatalf("partial output was not durable while provider still running %#v", partial)
	}
	events, err := f.s.Events(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var deltaID int64
	for _, event := range events {
		if event.Kind == "delta" {
			if !strings.Contains(string(event.Data), "林川") {
				t.Fatal("wrong delta")
			}
			deltaID = event.ID
		}
	}
	if deltaID == 0 {
		t.Fatal("increment was published before event persistence")
	}
	close(finish)
	select {
	case err = <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not complete")
	}
	saved, err := f.s.GetRun(ctx, run.ID)
	if err != nil || saved.Status != "Succeeded" || saved.Output.Text != "林川走进磨坊。" {
		t.Fatalf("final output %#v %v", saved, err)
	}
	replay, err := f.s.Events(ctx, run.ID, deltaID)
	if err != nil {
		t.Fatal(err)
	}
	tail := ""
	for _, event := range replay {
		if event.Kind == "delta" {
			tail += string(event.Data)
		}
	}
	if strings.Contains(tail, "林川") || !strings.Contains(tail, "走进磨坊") {
		t.Fatalf("cursor replay duplicated or lost increments %s", tail)
	}
	revision, err := f.s.GetRevision(ctx, chapter.CurrentRevisionID)
	if err != nil || revision.PlainText != "" {
		t.Fatal("stream overwrote author document")
	}
	if err = f.s.persistRunDelta(ctx, claimed, llm.Delta{Text: "迟到增量"}, "迟到覆盖"); err == nil {
		t.Fatal("terminal job accepted old worker increments")
	}
}

func TestWorkerStreamPersistenceFailureStopsProviderAndNeedsReconciliation(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter, err := f.s.CreateChapter(ctx, f.p.ID, "第一章", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.UpdateChapter(ctx, chapter.ID, "第一章", "", "雨夜进入磨坊"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(ctx, `CREATE FUNCTION reject_delta_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='delta' THEN RAISE EXCEPTION 'injected stream persistence failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_delta_event BEFORE INSERT ON wb_run_events FOR EACH ROW EXECUTE FUNCTION reject_delta_event()`); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"保留部分\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"不能继续提交\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	provider, err := llm.NewCompatible(llm.Config{BaseURL: server.URL, APIKey: "not-a-real-key", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	f.s.Provider = provider
	f.s.Model = "fake-stream-model"
	f.s.ProviderName = "fake-stream"
	run, err := f.s.SubmitRun(ctx, chapter.ID, RunRequest{IntentID: "failed-stream-save", TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := f.s.ClaimRun(ctx, "failed-stream-worker")
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	if err = f.s.ExecuteRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	saved, err := f.s.GetRun(ctx, run.ID)
	if err != nil || saved.Status != "NeedsReconciliation" || saved.Output.Text != "保留部分" || calls.Load() != 1 {
		t.Fatalf("stream continued or retried %#v calls=%d %v", saved, calls.Load(), err)
	}
	events, err := f.s.Events(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind == "delta" {
			t.Fatal("failed persistence exposed a phantom delta")
		}
	}
	if _, err = f.s.RunAction(ctx, run.ID, "retry"); !IsConflict(err) {
		t.Fatalf("unknown stream result retried %v", err)
	}
}

func TestWorkerBatchesThousandActualDeltasWithoutLossOnCompleteOrEOF(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(fmt.Sprint("complete-", complete), func(t *testing.T) {
			f := newNarrativeFixture(t)
			ctx := context.Background()
			chapter, err := f.s.CreateChapter(ctx, f.p.ID, "第一章", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.UpdateChapter(ctx, chapter.ID, "第一章", "", "千字增量批量保存"); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for i := 0; i < 1000; i++ {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"字\"}}]}\n\n")
				}
				if complete {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			provider, err := llm.NewCompatible(llm.Config{BaseURL: server.URL, APIKey: "not-a-real-key", Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			f.s.Provider = provider
			f.s.Model = "fake-stream-model"
			f.s.ProviderName = "fake-stream"
			run, err := f.s.SubmitRun(ctx, chapter.ID, RunRequest{IntentID: "batch-one-thousand", TaskType: "draft"})
			if err != nil {
				t.Fatal(err)
			}
			claimed, ok, err := f.s.ClaimRun(ctx, "batch-stream-worker")
			if err != nil || !ok {
				t.Fatalf("claim %v %v", ok, err)
			}
			if err = f.s.ExecuteRun(ctx, claimed); err != nil {
				t.Fatal(err)
			}
			saved, err := f.s.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := strings.Repeat("字", 1000)
			if saved.Output.Text != expected {
				t.Fatalf("batched output lost bytes: got %d want %d", len(saved.Output.Text), len(expected))
			}
			if complete && saved.Status != "Succeeded" || !complete && saved.Status != "NeedsReconciliation" {
				t.Fatalf("wrong completion status %s", saved.Status)
			}
			events, err := f.s.Events(ctx, run.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			var joined strings.Builder
			count := 0
			var firstID int64
			for _, event := range events {
				if event.Kind == "delta" {
					var delta llm.Delta
					if err = json.Unmarshal(event.Data, &delta); err != nil {
						t.Fatal(err)
					}
					joined.WriteString(delta.Text)
					count++
					if firstID == 0 {
						firstID = event.ID
						if delta.Text != "字" {
							t.Fatal("first increment should flush immediately")
						}
					}
				}
			}
			if joined.String() != expected || count > 100 || count < 2 {
				t.Fatalf("batched SSE duplicated/lost text or remained per-token: events=%d bytes=%d", count, joined.Len())
			}
			replay, err := f.s.Events(ctx, run.ID, firstID)
			if err != nil {
				t.Fatal(err)
			}
			var remainder strings.Builder
			for _, event := range replay {
				if event.Kind == "delta" {
					var delta llm.Delta
					_ = json.Unmarshal(event.Data, &delta)
					remainder.WriteString(delta.Text)
				}
			}
			if remainder.String() != strings.TrimPrefix(expected, "字") {
				t.Fatal("cursor replay changed batched content")
			}
			t.Logf("1000 provider increments persisted as %d durable delta events (complete=%v)", count, complete)
		})
	}
}

func TestWorkerBatchTimerFlushesPendingAndCancelsSilentStream(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter, err := f.s.CreateChapter(ctx, f.p.ID, "第一章", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.UpdateChapter(ctx, chapter.ID, "第一章", "", "短增量后等待"); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	disconnected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"首\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"待保存尾段\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(ready)
		select {
		case <-r.Context().Done():
			close(disconnected)
		case <-time.After(3 * time.Second):
		}
	}))
	defer server.Close()
	provider, err := llm.NewCompatible(llm.Config{BaseURL: server.URL, APIKey: "not-a-real-key", Timeout: 4 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	f.s.Provider = provider
	f.s.Model = "fake-stream-model"
	f.s.ProviderName = "fake-stream"
	run, err := f.s.SubmitRun(ctx, chapter.ID, RunRequest{IntentID: "batch-timer", TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := f.s.ClaimRun(ctx, "batch-timer-worker")
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	completed := make(chan error, 1)
	go func() { completed <- f.s.ExecuteRun(ctx, claimed) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("stream never started")
	}
	deadline := time.Now().Add(time.Second)
	var saved Run
	for time.Now().Before(deadline) {
		saved, err = f.s.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Output.Text == "首待保存尾段" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if saved.Output.Text != "首待保存尾段" || saved.Status != "Running" {
		t.Fatalf("timer did not flush pending bytes %#v", saved)
	}
	if _, err = f.s.RunAction(ctx, run.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-disconnected:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("silent provider was not cancelled by the batch timer")
	}
	select {
	case err = <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled stream did not finish")
	}
	saved, err = f.s.GetRun(ctx, run.ID)
	if err != nil || saved.Status != "Cancelled" || saved.Output.Text != "首待保存尾段" {
		t.Fatalf("cancel lost batched output %#v %v", saved, err)
	}
}
