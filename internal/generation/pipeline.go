package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"novel/internal/evaluation"
	"novel/internal/job"
	"novel/internal/llm"
	"novel/internal/review"
)

type Config struct {
	Store                                                     *job.Store
	Fixture                                                   *evaluation.Fixture
	Provider                                                  llm.Provider
	IntentID, ProviderName, Model, WriterPrompt, ReviewPrompt string
	// Endpoint and WireAPI are non-secret parts of the reproducible configuration.
	Endpoint, WireAPI string
	ReviewSchema      []byte
	MaxOutputTokens   int
}

func Run(ctx context.Context, c Config) (Report, error) {
	if c.Store == nil || c.Fixture == nil || c.Provider == nil || c.IntentID == "" || c.Model == "" || c.MaxOutputTokens < 1 {
		return Report{}, fmt.Errorf("incomplete pipeline configuration")
	}
	prefix, err := c.Fixture.Prefix("03")
	if err != nil {
		return Report{}, err
	}
	var outline string
	for _, ch := range c.Fixture.Chapters {
		if ch.ID == "03" {
			outline = ch.Outline
		}
	}
	// The snapshot deliberately contains neither the answer key nor chapter 03's
	// canonical text. Frozen candidate cases are reviewed in separate requests.
	input := struct {
		FixtureVersion, Story, Prefix, Outline, CorrectDraft, IncorrectDraft string
		WriterPrompt, ReviewPrompt, Model, Provider, Endpoint, WireAPI       string
		Schema                                                               json.RawMessage
		MaxOutputTokens                                                      int
	}{c.Fixture.Version, c.Fixture.Story, prefix, outline, c.Fixture.CorrectDraft, c.Fixture.IncorrectDraft, c.WriterPrompt, c.ReviewPrompt, c.Model, c.ProviderName, c.Endpoint, c.WireAPI, c.ReviewSchema, c.MaxOutputTokens}
	snapshot, err := json.Marshal(input)
	if err != nil {
		return Report{}, err
	}
	j, created, err := c.Store.Prepare(ctx, job.Intent{ProjectKey: c.Fixture.Version, IntentID: c.IntentID, InputHash: job.Hash(snapshot), Input: snapshot})
	if err != nil {
		return Report{}, err
	}
	if !created {
		artifacts, err := c.Store.Artifacts(ctx, j.ID)
		if err != nil {
			return Report{}, err
		}
		for _, a := range artifacts {
			if a.Kind == "report" {
				var old Report
				if err = json.Unmarshal(a.Content, &old); err != nil {
					return old, err
				}
				if old.Status != "Succeeded" {
					return old, fmt.Errorf("existing intent ended as %s; no request was dispatched", old.Status)
				}
				return old, nil
			}
		}
		return Report{RunID: j.ID, IntentID: c.IntentID, Status: j.Status}, fmt.Errorf("intent already exists without a completed report; inspect attempts before recovery")
	}
	r := Report{ExperimentID: "bridge-prefix-" + j.ID, RunID: j.ID, IntentID: c.IntentID, FixtureVersion: c.Fixture.Version, Configuration: "A-prefix-diagnostic", Provider: c.ProviderName, Model: c.Model, InputHash: j.InputHash, Status: "Running", StartedAt: time.Now().UTC(), Samples: []Sample{}, CostStatus: "Unknown", Notes: []string{"Stage 00 uses complete chapters 01/02 as diagnostic context, not the later A/B/C comparison.", "No provider pricing was configured; unknown monetary cost is null.", "Issue metrics and author editing time require independent annotation."}}
	if c.ProviderName == "fake" {
		r.Notes = append(r.Notes, "Deterministic fake responses validate engineering only; they do not measure model quality.")
	}
	finish := func(status string, cause error) (Report, error) {
		r.Status = status
		r.FinishedAt = time.Now().UTC()
		data, err := json.Marshal(r)
		if err != nil {
			return r, err
		}
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = c.Store.Complete(saveCtx, j.ID, status, data); err != nil {
			return r, fmt.Errorf("persist final report: %w", err)
		}
		return r, cause
	}
	call := func(step, promptVersion string, req llm.Request, isReview bool) (string, error) {
		request, err := json.Marshal(req)
		if err != nil {
			return "", err
		}
		a, err := c.Store.BeginAttempt(ctx, j.ID, job.Call{Step: step, Provider: c.ProviderName, Model: c.Model, PromptVersion: promptVersion, Request: request, PriceSnapshot: []byte(`{"status":"Unknown","currency":null,"inputPerMillion":null,"outputPerMillion":null}`)})
		if err != nil {
			return "", err
		}
		started := time.Now()
		res, callErr := c.Provider.Generate(ctx, req)
		sample := Sample{Step: step, AttemptID: a.ID, PromptVersion: promptVersion, Outcome: "Succeeded", Text: res.Text, ProviderRequestID: res.ProviderRequestID, DurationMs: time.Since(started).Milliseconds()}
		if res.Usage.Reported {
			sample.InputTokens = &res.Usage.InputTokens
			sample.OutputTokens = &res.Usage.OutputTokens
			sample.CachedInputTokens = &res.Usage.CachedInputTokens
		}
		if callErr != nil {
			sample.Outcome = "Failed"
			if llm.FailureDisposition(callErr, true) == llm.NeedsReconciliation {
				sample.Outcome = "NeedsReconciliation"
			}
			// Do not copy arbitrary provider error strings into logs or reports.
			sample.Error = "provider call failed: " + llm.FailureDisposition(callErr, true)
		} else if isReview {
			sample.Issues, err = review.DecodeIssues([]byte(res.Text))
			// Validate against the decoded candidate alone, never against the historical prefix.
			if err == nil {
				var document struct {
					Candidate string `json:"candidate"`
				}
				if e := json.Unmarshal([]byte(req.Messages[len(req.Messages)-1].Content), &document); e != nil {
					err = e
				} else {
					err = review.ValidateIssues(document.Candidate, sample.Issues)
				}
			}
			if err != nil {
				callErr = fmt.Errorf("invalid review: %w", err)
				sample.Outcome = "Failed"
				sample.Error = callErr.Error()
			}
		} else if res.Text == "" {
			callErr = fmt.Errorf("empty draft")
			sample.Outcome = "Failed"
			sample.Error = "empty draft"
		}
		r.Samples = append(r.Samples, sample)
		usage, _ := json.Marshal(res.Usage)
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = c.Store.FinishAttempt(saveCtx, a, job.Completion{Outcome: sample.Outcome, Raw: res.Raw, Usage: usage, ProviderRequestID: res.ProviderRequestID, Error: sample.Error}); err != nil {
			// The provider may already have completed; a failed local commit
			// cannot turn that external result into an ordinary safe-to-retry
			// failure. Preserve the dispatched attempt for explicit inspection.
			sample.Outcome = "NeedsReconciliation"
			sample.Error = "response persistence failed; inspect attempt before recovery"
			r.Samples[len(r.Samples)-1] = sample
			return "", fmt.Errorf("persist response; inspect attempt before recovery: %w", err)
		}
		artifact, _ := json.Marshal(sample)
		if err = c.Store.AddArtifact(saveCtx, j.ID, step, artifact); err != nil {
			return "", err
		}
		if callErr != nil {
			return "", callErr
		}
		return res.Text, nil
	}
	userInput, _ := json.Marshal(map[string]string{"confirmedSetting": c.Fixture.Story, "previousChapters": prefix, "outline": outline, "authorRequest": "起草第三章候选，约400至600汉字，停在开箱之前。"})
	draft, err := call("write-03", "chapter-v1", llm.Request{Model: c.Model, Messages: []llm.Message{{Role: "system", Content: c.WriterPrompt}, {Role: "user", Content: string(userInput)}}, MaxOutputTokens: c.MaxOutputTokens}, false)
	if err != nil {
		return finish(failureStatus(r), err)
	}
	cases := []struct{ ID, Text string }{{"generated", draft}, {"correct", c.Fixture.CorrectDraft}, {"incorrect", c.Fixture.IncorrectDraft}}
	for _, candidate := range cases {
		userInput, _ := json.Marshal(map[string]string{"confirmedSetting": c.Fixture.Story, "previousChapters": prefix, "candidate": candidate.Text})
		_, err = call("review-"+candidate.ID, "continuity-v1", llm.Request{Model: c.Model, Messages: []llm.Message{{Role: "system", Content: c.ReviewPrompt}, {Role: "user", Content: string(userInput)}}, MaxOutputTokens: c.MaxOutputTokens, ResponseSchema: c.ReviewSchema}, true)
		if err != nil {
			return finish(failureStatus(r), err)
		}
	}
	return finish("Succeeded", nil)
}

func failureStatus(r Report) string {
	if len(r.Samples) > 0 && r.Samples[len(r.Samples)-1].Outcome == "NeedsReconciliation" {
		return "NeedsReconciliation"
	}
	return "Failed"
}
