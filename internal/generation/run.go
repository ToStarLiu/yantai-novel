package generation

import (
	"novel/internal/review"
	"time"
)

type Sample struct {
	Step              string         `json:"step"`
	AttemptID         string         `json:"attemptId"`
	PromptVersion     string         `json:"promptVersion"`
	Outcome           string         `json:"outcome"`
	Text              string         `json:"text"`
	Issues            []review.Issue `json:"issues,omitempty"`
	ProviderRequestID string         `json:"providerRequestId,omitempty"`
	InputTokens       *int64         `json:"inputTokens"`
	OutputTokens      *int64         `json:"outputTokens"`
	CachedInputTokens *int64         `json:"cachedInputTokens"`
	DurationMs        int64          `json:"durationMs"`
	Error             string         `json:"error,omitempty"`
}

type Report struct {
	ExperimentID         string    `json:"experimentId"`
	RunID                string    `json:"runId"`
	IntentID             string    `json:"intentId"`
	FixtureVersion       string    `json:"fixtureVersion"`
	Configuration        string    `json:"configuration"`
	Provider             string    `json:"provider"`
	Model                string    `json:"model"`
	InputHash            string    `json:"inputHash"`
	Status               string    `json:"status"`
	StartedAt            time.Time `json:"startedAt"`
	FinishedAt           time.Time `json:"finishedAt"`
	Samples              []Sample  `json:"samples"`
	EstimatedCost        *float64  `json:"estimatedCost"`
	Currency             *string   `json:"currency"`
	CostStatus           string    `json:"costStatus"`
	IssueTruePositive    *int      `json:"issueTruePositive"`
	IssueFalsePositive   *int      `json:"issueFalsePositive"`
	IssueFalseNegative   *int      `json:"issueFalseNegative"`
	AuthorEditingMinutes *float64  `json:"authorEditingMinutes"`
	Notes                []string  `json:"notes"`
}
