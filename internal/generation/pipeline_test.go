package generation

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"novel/internal/evaluation"
	"novel/internal/job"
	"novel/internal/llm"
	"novel/migrations"
)

func integration(t *testing.T) (*job.Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + job.NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, _ := pgxpool.ParseConfig(url)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	if err = migrations.Apply(ctx, p, "up"); err != nil {
		t.Fatal(err)
	}
	return job.NewStore(p), p
}

func config(t *testing.T, s *job.Store) Config {
	t.Helper()
	f, err := evaluation.Load("../../evaluation/fixtures/bridge-3")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := os.ReadFile("../../prompts/writing/chapter-v1.md")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := os.ReadFile("../../prompts/review/continuity-v1.md")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("../../schemas/review-issues-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return Config{Store: s, Fixture: f, IntentID: job.NewID(), ProviderName: "fake", Model: "fake-v1", WriterPrompt: string(writer), ReviewPrompt: string(reviewer), ReviewSchema: schema, MaxOutputTokens: 1600}
}

func TestPipelinePersistsArtifactsAndReplaysIntentWithoutDispatch(t *testing.T) {
	s, p := integration(t)
	c := config(t, s)
	calls := 0
	c.Provider = &llm.Fake{Handler: func(ctx context.Context, r llm.Request) (llm.Response, error) {
		calls++
		if strings.Contains(r.Messages[1].Content, "evaluationOnly") {
			t.Fatal("answer key leaked")
		}
		var runningID string
		if err := p.QueryRow(ctx, "SELECT id FROM generation_jobs WHERE intent_id=$1", c.IntentID).Scan(&runningID); err != nil {
			t.Fatal(err)
		}
		attempts, err := s.Attempts(ctx, runningID)
		if err != nil || len(attempts) != calls {
			t.Fatalf("attempt not committed before dispatch: %v", err)
		}
		text := c.Fixture.CorrectDraft
		if len(r.ResponseSchema) > 0 {
			text = `{"issues":[]}`
		}
		return llm.Response{Text: text, Raw: []byte(text)}, nil
	}}
	r, err := Run(context.Background(), c)
	if err != nil || r.Status != "Succeeded" {
		t.Fatalf("run: %v %s", err, r.Status)
	}
	if calls != 4 || len(r.Samples) != 4 {
		t.Fatalf("calls=%d samples=%d", calls, len(r.Samples))
	}
	if r.Samples[0].InputTokens != nil || r.EstimatedCost != nil {
		t.Fatal("unknown usage or cost must remain null")
	}
	again, err := Run(context.Background(), c)
	if err != nil || again.RunID != r.RunID || calls != 4 {
		t.Fatalf("replay dispatched: %v calls=%d", err, calls)
	}
	artifacts, err := s.Artifacts(context.Background(), r.RunID)
	if err != nil || len(artifacts) < 9 {
		t.Fatalf("missing artifacts %d %v", len(artifacts), err)
	}
	c.MaxOutputTokens++
	if _, err = Run(context.Background(), c); err == nil {
		t.Fatal("changed inputs with same intent must fail")
	}
}

func TestPipelineUnknownResponseStopsAndRetainsFailure(t *testing.T) {
	s, _ := integration(t)
	c := config(t, s)
	calls := 0
	c.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		return llm.Response{Raw: []byte("partial-output")}, errors.New("connection lost after send")
	}}
	r, err := Run(context.Background(), c)
	if err == nil || r.Status != "NeedsReconciliation" || calls != 1 {
		t.Fatalf("unknown: %s %d %v", r.Status, calls, err)
	}
	j, err := s.Get(context.Background(), r.RunID)
	if err != nil || j.Status != "NeedsReconciliation" {
		t.Fatalf("status not durable %v", err)
	}
	artifacts, err := s.Artifacts(context.Background(), r.RunID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range artifacts {
		if string(a.Content) == "partial-output" {
			found = true
		}
	}
	if !found {
		t.Fatal("failed raw output missing")
	}
	_, _ = Run(context.Background(), c)
	if calls != 1 {
		t.Fatal("unknown result retried")
	}
}

func TestPipelineRejectsQuoteFromHistoryOutsideCandidate(t *testing.T) {
	s, _ := integration(t)
	c := config(t, s)
	calls := 0
	c.Provider = &llm.Fake{Handler: func(_ context.Context, r llm.Request) (llm.Response, error) {
		calls++
		text := "这是与历史引文不同的候选正文。"
		if len(r.ResponseSchema) > 0 {
			text = `{"issues":[{"kind":"Continuity","severity":"Major","quote":"木刺划开了他的左臂。","explanation":"历史引文不可以冒充候选问题位置","suggestedAction":"核对引用"}]}`
		}
		return llm.Response{Text: text, Raw: []byte(text)}, nil
	}}
	r, err := Run(context.Background(), c)
	if err == nil || r.Status != "Failed" || calls != 2 {
		t.Fatalf("historical quote accepted: status=%s calls=%d err=%v", r.Status, calls, err)
	}
	if !strings.Contains(r.Samples[1].Error, "evidence quote is absent") {
		t.Fatalf("wrong validation failure: %s", r.Samples[1].Error)
	}
}

func TestPipelineResponsePersistenceFailureNeedsReconciliation(t *testing.T) {
	s, p := integration(t)
	c := config(t, s)
	ctx := context.Background()
	// Allow the final report to commit, but simulate failure at the exact window
	// after a provider has completed and before its raw response is durable.
	if _, err := p.Exec(ctx, `
		CREATE FUNCTION reject_provider_raw() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.kind = 'provider-raw' THEN
				RAISE EXCEPTION 'injected raw-response persistence failure';
			END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER reject_provider_raw BEFORE INSERT ON run_artifacts
		FOR EACH ROW EXECUTE FUNCTION reject_provider_raw();
	`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		return llm.Response{Text: "已完成但尚未持久保存的正文。", Raw: []byte("provider-completed"), ProviderRequestID: "provider-test"}, nil
	}}
	r, err := Run(ctx, c)
	if err == nil || r.Status != "NeedsReconciliation" || calls != 1 {
		t.Fatalf("response persistence failure must remain unknown: status=%s calls=%d err=%v", r.Status, calls, err)
	}
	if len(r.Samples) != 1 || r.Samples[0].Outcome != "NeedsReconciliation" || !strings.Contains(r.Samples[0].Error, "response persistence failed") {
		t.Fatalf("sample does not explain the unknown persistence window: %+v", r.Samples)
	}
	j, err := s.Get(ctx, r.RunID)
	if err != nil || j.Status != "NeedsReconciliation" {
		t.Fatalf("unknown business status must be durable: %+v %v", j, err)
	}
	attempts, err := s.Attempts(ctx, r.RunID)
	if err != nil || len(attempts) != 1 || attempts[0].Outcome != "Running" {
		t.Fatalf("failed transaction should leave the dispatched attempt for inspection: %+v %v", attempts, err)
	}
	_, _ = Run(ctx, c)
	if calls != 1 {
		t.Fatal("response persistence failure must never dispatch the intent again")
	}
}
