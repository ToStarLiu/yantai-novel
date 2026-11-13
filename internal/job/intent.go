package job

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"novel/internal/dbgen"
)

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func Hash(input []byte) string { sum := sha256.Sum256(input); return hex.EncodeToString(sum[:]) }

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Intent struct {
	ProjectKey, IntentID, InputHash string
	Input                           []byte
}
type Job = dbgen.GenerationJob
type Call struct {
	Step, Provider, Model, PromptVersion string
	Request, PriceSnapshot               []byte
}
type Attempt struct{ ID, JobID string }
type Completion struct {
	Outcome, ProviderRequestID, Error string
	Raw, Usage                        []byte
}

func (s *Store) Prepare(ctx context.Context, in Intent) (Job, bool, error) {
	if in.ProjectKey == "" || in.IntentID == "" || in.InputHash == "" || !json.Valid(in.Input) {
		return Job{}, false, fmt.Errorf("invalid intent")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback(ctx)
	q := dbgen.New(tx)
	projectID, err := q.EnsureProject(ctx, dbgen.EnsureProjectParams{ID: NewID(), ProjectKey: in.ProjectKey})
	if err != nil {
		return Job{}, false, err
	}
	n, err := q.InsertJob(ctx, dbgen.InsertJobParams{ID: NewID(), ProjectID: projectID, IntentID: in.IntentID, InputHash: in.InputHash, InputSnapshot: in.Input})
	if err != nil {
		return Job{}, false, err
	}
	j, err := q.GetJobByIntent(ctx, in.IntentID)
	if err != nil {
		return Job{}, false, err
	}
	if j.ProjectID != projectID || j.InputHash != in.InputHash {
		return Job{}, false, fmt.Errorf("intent already exists with different project or inputs")
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, false, err
	}
	return j, n == 1, nil
}

// BeginAttempt commits before its caller is permitted to dispatch external work.
func (s *Store) BeginAttempt(ctx context.Context, jobID string, c Call) (Attempt, error) {
	if c.Step == "" || !json.Valid(c.Request) || !json.Valid(c.PriceSnapshot) {
		return Attempt{}, fmt.Errorf("invalid call snapshot")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Attempt{}, err
	}
	defer tx.Rollback(ctx)
	q := dbgen.New(tx)
	j, err := q.LockJob(ctx, jobID)
	if err != nil {
		return Attempt{}, err
	}
	if j.Status != "Pending" && j.Status != "Running" {
		return Attempt{}, fmt.Errorf("job is %s; dispatch refused", j.Status)
	}
	a := Attempt{ID: NewID(), JobID: jobID}
	if err = q.InsertAttempt(ctx, dbgen.InsertAttemptParams{ID: a.ID, JobID: jobID, Step: c.Step, InputHash: Hash(c.Request), PromptVersion: c.PromptVersion, Provider: c.Provider, Model: c.Model, RequestSnapshot: c.Request, PriceSnapshot: c.PriceSnapshot}); err != nil {
		return Attempt{}, err
	}
	if err = q.StartJob(ctx, jobID); err != nil {
		return Attempt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Attempt{}, err
	}
	return a, nil
}

func (s *Store) FinishAttempt(ctx context.Context, a Attempt, c Completion) error {
	if c.Outcome != "Succeeded" && c.Outcome != "Failed" && c.Outcome != "NeedsReconciliation" {
		return fmt.Errorf("invalid attempt outcome")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := dbgen.New(tx)
	artifact := NewID()
	raw := c.Raw
	if raw == nil {
		raw = []byte{}
	}
	if err = q.InsertArtifact(ctx, dbgen.InsertArtifactParams{ID: artifact, JobID: a.JobID, Kind: "provider-raw", Content: raw}); err != nil {
		return err
	}
	usage := c.Usage
	if usage == nil {
		usage = []byte(`{"reported":false}`)
	}
	n, err := q.FinishAttempt(ctx, dbgen.FinishAttemptParams{ID: a.ID, Outcome: c.Outcome, RawArtifactID: nullableText(artifact), ProviderRequestID: c.ProviderRequestID, Usage: usage, Error: c.Error, JobID: a.JobID})
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("attempt already completed or does not belong to job")
	}
	return tx.Commit(ctx)
}

func (s *Store) Complete(ctx context.Context, id, status string, report []byte) error {
	if status != "Succeeded" && status != "Failed" && status != "NeedsReconciliation" {
		return fmt.Errorf("invalid final status")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := dbgen.New(tx)
	n, err := q.CompleteJob(ctx, dbgen.CompleteJobParams{ID: id, Status: status})
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("job already completed")
	}
	if err = q.InsertArtifact(ctx, dbgen.InsertArtifactParams{ID: NewID(), JobID: id, Kind: "report", Content: report}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AddArtifact(ctx context.Context, id, kind string, content []byte) error {
	return dbgen.New(s.pool).InsertArtifact(ctx, dbgen.InsertArtifactParams{ID: NewID(), JobID: id, Kind: kind, Content: content})
}
func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	return dbgen.New(s.pool).GetJob(ctx, id)
}
func (s *Store) Artifacts(ctx context.Context, id string) ([]dbgen.RunArtifact, error) {
	return dbgen.New(s.pool).ListArtifacts(ctx, id)
}
func (s *Store) Attempts(ctx context.Context, id string) ([]dbgen.JobAttempt, error) {
	return dbgen.New(s.pool).ListAttempts(ctx, id)
}
