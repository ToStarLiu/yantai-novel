package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"novel/internal/llm"
	"time"
)

var ErrNotFound = errors.New("未找到记录")

type Conflict struct {
	Message           string
	CurrentRevisionID string `json:"currentRevisionId,omitempty"`
}

func (e *Conflict) Error() string { return e.Message }
func IsConflict(err error) bool   { var c *Conflict; return errors.As(err, &c) }

type Service struct {
	MediaConfig     MediaConfig
	DB              *pgxpool.Pool
	Provider        llm.Provider
	Model           string
	ProviderName    string
	MaxOutputTokens int
	MaxConcurrent   int
	JobTimeout      time.Duration
	ModelPolicies   map[string]ModelPolicy
	MaxJobCost      *float64
	Currency        string
}
type Project struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Genre          string    `json:"genre"`
	Epoch          int64     `json:"epoch"`
	Health         string    `json:"health"`
	StateVersionID string    `json:"stateVersionId"`
	CreatedAt      time.Time `json:"createdAt"`
}
type Setting struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Content   string `json:"content"`
}
type Chapter struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"projectId"`
	Volume            string    `json:"volume"`
	Title             string    `json:"title"`
	Order             int       `json:"order"`
	Blueprint         string    `json:"blueprint"`
	BlueprintVersion  int64     `json:"blueprintVersion"`
	CurrentRevisionID string    `json:"currentRevisionId"`
	FinalRevisionID   string    `json:"finalRevisionId"`
	NeedsReview       bool      `json:"needsReview"`
	Deleted           bool      `json:"deleted"`
	HasText           *bool     `json:"hasText,omitempty"`
	Revision          *Revision `json:"revision,omitempty"`
}
type Revision struct {
	ID        string          `json:"id"`
	ChapterID string          `json:"chapterId"`
	ParentID  string          `json:"parentId"`
	Document  json.RawMessage `json:"document"`
	PlainText string          `json:"plainText"`
	Hash      string          `json:"hash"`
	Source    string          `json:"source"`
	RunID     string          `json:"runId"`
	CreatedAt time.Time       `json:"createdAt"`
}
type ProjectDetail struct {
	Project  Project   `json:"project"`
	Chapters []Chapter `json:"chapters"`
	Settings []Setting `json:"settings"`
}
type SaveInput struct {
	ExpectedCurrentRevisionID string          `json:"expectedCurrentRevisionId"`
	Document                  json.RawMessage `json:"document"`
	Source                    string          `json:"source"`
	RunID                     string          `json:"runId"`
}
type Impact struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"projectId"`
	EarliestOrder int       `json:"earliestOrder"`
	Reason        string    `json:"reason"`
	ChapterIDs    []string  `json:"chapterIds"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Core methods are implemented in core.go. Narrative and task modules share
// the same PostgreSQL transaction boundary through this service.
type Core interface {
	GetProject(context.Context, string) (ProjectDetail, error)
	GetChapter(context.Context, string) (Chapter, error)
	GetRevision(context.Context, string) (Revision, error)
}
