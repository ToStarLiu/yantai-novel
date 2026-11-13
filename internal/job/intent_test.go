package job

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"novel/migrations"
)

func testStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	if err := migrations.Apply(ctx, pool, "up"); err != nil {
		t.Fatal(err)
	}
	return NewStore(pool), pool
}

func TestIntentDeduplicationAndInputConflict(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	in := Intent{ProjectKey: "bridge-3", IntentID: NewID(), InputHash: "hash-a", Input: []byte(`{"outline":"test"}`)}
	first, created, err := s.Prepare(ctx, in)
	if err != nil || !created {
		t.Fatalf("first: %v %v", created, err)
	}
	second, created, err := s.Prepare(ctx, in)
	if err != nil || created || first.ID != second.ID {
		t.Fatalf("duplicate: %v %v", created, err)
	}
	in.InputHash = "hash-b"
	if _, _, err = s.Prepare(ctx, in); err == nil {
		t.Fatal("same intent with different inputs must fail")
	}
}

func TestConcurrentPrepareCreatesOneJob(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	in := Intent{ProjectKey: "bridge-3", IntentID: NewID(), InputHash: "same-hash", Input: []byte(`{"outline":"same request"}`)}
	type result struct {
		job     Job
		created bool
		err     error
	}
	const count = 8
	start := make(chan struct{})
	results := make(chan result, count)
	for i := 0; i < count; i++ {
		go func() {
			<-start
			j, created, err := s.Prepare(ctx, in)
			results <- result{job: j, created: created, err: err}
		}()
	}
	close(start)
	jobID := ""
	createdCount := 0
	for i := 0; i < count; i++ {
		got := <-results
		if got.err != nil {
			t.Fatalf("concurrent Prepare failed: %v", got.err)
		}
		if jobID == "" {
			jobID = got.job.ID
		}
		if got.job.ID != jobID {
			t.Fatalf("same intent created different jobs: %s and %s", jobID, got.job.ID)
		}
		if got.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("expected one creator, got %d", createdCount)
	}
}

func TestAttemptCannotAttachArtifactToAnotherJob(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	newAttempt := func(project string) Attempt {
		t.Helper()
		j, _, err := s.Prepare(ctx, Intent{ProjectKey: project, IntentID: NewID(), InputHash: "hash", Input: []byte(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		a, err := s.BeginAttempt(ctx, j.ID, Call{Step: "write-03", Provider: "fake", Model: "fake-v1", PromptVersion: "chapter-v1", Request: []byte(`{}`), PriceSnapshot: []byte(`{"status":"Unknown"}`)})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	first := newAttempt("first-project")
	second := newAttempt("second-project")
	err := s.FinishAttempt(ctx, Attempt{ID: first.ID, JobID: second.JobID}, Completion{Outcome: "Succeeded", Raw: []byte("must not cross projects")})
	if err == nil {
		t.Fatal("attempt ownership mismatch must be rejected")
	}
	var artifacts int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM run_artifacts").Scan(&artifacts); err != nil {
		t.Fatal(err)
	}
	if artifacts != 0 {
		t.Fatalf("failed ownership check left %d artifacts behind", artifacts)
	}
	for _, attempt := range []Attempt{first, second} {
		var status string
		var rawArtifact *string
		if err := p.QueryRow(ctx, "SELECT outcome, raw_artifact_id FROM job_attempts WHERE id=$1", attempt.ID).Scan(&status, &rawArtifact); err != nil {
			t.Fatal(err)
		}
		if status != "Running" || rawArtifact != nil {
			t.Fatalf("mismatched finish changed attempt %s: status=%s raw=%v", attempt.ID, status, rawArtifact)
		}
	}
}

func TestAttemptCommittedBeforeExternalCallAndUnknownNotRetried(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	j, _, err := s.Prepare(ctx, Intent{ProjectKey: "bridge-3", IntentID: NewID(), InputHash: "hash", Input: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.BeginAttempt(ctx, j.ID, Call{Step: "write-03", Provider: "fake", Model: "fake-v1", PromptVersion: "chapter-v1", Request: []byte(`{}`), PriceSnapshot: []byte(`{"status":"Unknown"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if err = p.QueryRow(ctx, "SELECT outcome FROM job_attempts WHERE id=$1", a.ID).Scan(&status); err != nil || status != "Running" {
		t.Fatalf("attempt must be visible on independent connection: %q %v", status, err)
	}
	if err = s.FinishAttempt(ctx, a, Completion{Outcome: "NeedsReconciliation", Raw: []byte("partial"), Usage: []byte(`{"reported":false}`), Error: "result unknown"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginAttempt(ctx, j.ID, Call{Step: "write-03", Provider: "fake", Model: "fake-v1", Request: []byte(`{}`), PriceSnapshot: []byte(`{}`)}); err == nil {
		t.Fatal("unknown attempt must not be dispatched twice")
	}
	var raw string
	if err = p.QueryRow(ctx, "SELECT convert_from(a.content,'UTF8') FROM run_artifacts a JOIN job_attempts t ON a.id=t.raw_artifact_id WHERE t.id=$1", a.ID).Scan(&raw); err != nil || raw != "partial" {
		t.Fatalf("raw failure artifact missing: %q %v", raw, err)
	}
}

func TestMigrationsRollbackAndReapply(t *testing.T) {
	_, p := testStore(t)
	ctx := context.Background()
	for _, d := range []string{"down", "up", "up"} {
		if err := migrations.Apply(ctx, p, d); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM projects").Scan(&count); err != nil || count != 0 {
		t.Fatalf("fresh migration: %d %v", count, err)
	}
}
