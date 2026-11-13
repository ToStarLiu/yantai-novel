package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"novel/internal/job"
	"novel/internal/llm"
	"novel/internal/review"
	"strings"
	"time"
)

// RecoverExpired never repeats a call that might have reached the provider.
func (s *Service) RecoverExpired(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT id FROM wb_runs WHERE status='Running' AND lease_until<now() FOR UPDATE SKIP LOCKED")
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		var calls int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM wb_run_attempts WHERE run_id=$1 AND status='Running'", id).Scan(&calls); err != nil {
			return err
		}
		status := "Pending"
		message := ""
		if calls > 0 {
			status = "NeedsReconciliation"
			message = "执行进程中断，调用结果未知；请核对后决定是否重新生成"
		}
		if _, err = tx.Exec(ctx, "UPDATE wb_runs SET status=$2,error=$3,lease_owner='',lease_until=NULL WHERE id=$1", id, status, message); err != nil {
			return err
		}
		if err = eventTx(ctx, tx, id, "status", map[string]string{"status": status, "error": message}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) ClaimRun(ctx context.Context, owner string) (Run, bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Run{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext(current_database()||current_schema()||'wb-provider-limit'))"); err != nil {
		return Run{}, false, err
	}
	limit := s.MaxConcurrent
	if limit < 1 {
		limit = 2
	}
	var active int
	if err = tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM wb_runs WHERE status='Running')+(SELECT count(*) FROM wb_copilot_turns WHERE status='Running')").Scan(&active); err != nil {
		return Run{}, false, err
	}
	if active >= limit {
		return Run{}, false, nil
	}
	models := []string{s.Model}
	for _, policy := range s.ModelRoutes() {
		models = append(models, policy.Model)
	}
	r, err := scanRun(tx.QueryRow(ctx, "SELECT "+runColumns+" FROM wb_runs r WHERE status IN ('Pending','RetryScheduled') AND ((input->>'provider'=$1 AND input->>'model'=ANY($2)) OR EXISTS(SELECT 1 FROM wb_provider_configs pc WHERE pc.id=input->>'provider' AND pc.enabled AND pc.model=input->>'model')) AND (intent_id NOT LIKE 'production:%' OR EXISTS(SELECT 1 FROM wb_productions p WHERE p.id=split_part(r.intent_id,':',2) AND p.status='Running')) AND next_attempt_at<=now() AND NOT EXISTS(SELECT 1 FROM wb_runs a WHERE a.project_id=r.project_id AND a.status='Running') ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED", s.ProviderName, models))
	if errors.Is(err, ErrNotFound) {
		return Run{}, false, nil
	}
	if err != nil {
		return r, false, err
	}
	timeout := s.JobTimeout
	if timeout <= 0 {
		timeout = 150 * time.Second
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_runs SET status='Running',lease_owner=$2,lease_generation=lease_generation+1,lease_until=now()+($3*interval '1 second') WHERE id=$1", r.ID, owner, int(timeout.Seconds()+30)); err != nil {
		return r, false, err
	}
	if err = eventTx(ctx, tx, r.ID, "status", map[string]string{"status": "Running"}); err != nil {
		return r, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return r, false, err
	}
	r, err = s.GetRun(ctx, r.ID)
	return r, true, err
}

func (s *Service) beginCall(ctx context.Context, r Run) (string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	current, err := snapshotCurrent(ctx, tx, r)
	if err != nil {
		return "", err
	}
	if !current {
		return "", &Conflict{Message: "输入已过期，请基于当前稿重新生成"}
	}
	if err = s.checkBudgetTx(ctx, tx, r.ProjectID, r.ID, r.Input); err != nil {
		return "", err
	}
	tag, err := tx.Exec(ctx, "UPDATE wb_runs SET attempt_count=attempt_count+1 WHERE id=$1 AND status='Running' AND lease_owner=$2 AND lease_generation=$3 AND lease_until>now()", r.ID, r.LeaseOwner, r.LeaseGeneration)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", fmt.Errorf("lease lost")
	}
	raw, _ := json.Marshal(r.Input.Request)
	id := job.NewID()
	if _, err = tx.Exec(ctx, "INSERT INTO wb_run_attempts(id,run_id,ordinal,status,request_snapshot) SELECT $1,id,attempt_count,'Running',$3 FROM wb_runs WHERE id=$2", id, r.ID, raw); err != nil {
		return "", err
	}
	if err = eventTx(ctx, tx, r.ID, "attempt", map[string]string{"attemptId": id}); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func (s *Service) ExecuteRun(ctx context.Context, r Run) error {
	if r.Input.UseCache && r.Type != "memory-extraction" {
		cached, ok, err := s.GetRunCache(ctx, r.Input)
		if err != nil {
			return err
		}
		if ok {
			return s.completeCachedRun(ctx, r, cached)
		}
	}
	provider, providerErr := s.ProviderByID(ctx, r.Input.Provider)
	if providerErr != nil {
		return s.failBeforeCall(ctx, r, fmt.Errorf("Provider配置不可用或密钥引用未配置"))
	}
	attempt, err := s.beginCall(ctx, r)
	if err != nil {
		return s.failBeforeCall(ctx, r, err)
	}
	timeout := s.JobTimeout
	if timeout <= 0 {
		timeout = 150 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var response llm.Response
	var callErr error
	if streaming, ok := provider.(llm.StreamingProvider); ok {
		response, callErr = s.streamRun(callCtx, r, streaming)
	} else {
		response, callErr = provider.Generate(callCtx, r.Input.Request)
	}
	output := RunOutput{Text: response.Text}
	disposition := ""
	status := "Succeeded"
	message := ""
	if callErr != nil {
		disposition = llm.FailureDisposition(callErr, true)
		status = "Failed"
		if disposition == llm.NeedsReconciliation {
			status = "NeedsReconciliation"
		}
		message = "模型调用失败：" + disposition
	}
	if callErr == nil && r.Type == "review" {
		output.Issues, err = review.DecodeIssues([]byte(response.Text))
		if err != nil {
			if repaired, changed := repairStructuredJSON(response.Text); changed {
				output.Issues, err = review.DecodeIssues([]byte(repaired))
				if err == nil {
					output.Text = repaired
					output.FormatRepaired = true
				}
			}
		}
		if err == nil {
			err = review.ValidateIssues(r.Input.Source, output.Issues)
		}
		if err != nil {
			status = "Failed"
			message = "审稿格式或证据校验失败"
			disposition = llm.Permanent
		}
	}
	var changes []MemoryChange
	if callErr == nil && r.Type == "memory-extraction" {
		changes, err = DecodeMemoryChanges([]byte(response.Text))
		if err != nil {
			if repaired, changed := repairStructuredJSON(response.Text); changed {
				changes, err = DecodeMemoryChanges([]byte(repaired))
				if err == nil {
					output.Text = repaired
					output.FormatRepaired = true
				}
			}
		}
		if err != nil {
			status = "Failed"
			message = "记忆提议格式校验失败"
			disposition = llm.Permanent
		}
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer saveCancel()
	// Response, derived memory proposal, events and terminal state are committed
	// together. A crash cannot expose success without its reviewable proposal.
	usage, _ := json.Marshal(response.Usage)
	tx, err := s.DB.Begin(saveCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(saveCtx)
	var cancelRequested, pauseRequested bool
	var generation int64
	var owner string
	if err = tx.QueryRow(saveCtx, "SELECT cancel_requested,pause_requested,lease_generation,lease_owner FROM wb_runs WHERE id=$1 AND status='Running' AND lease_until>now() FOR UPDATE", r.ID).Scan(&cancelRequested, &pauseRequested, &generation, &owner); err != nil {
		return err
	}
	if generation != r.LeaseGeneration || owner != r.LeaseOwner {
		return fmt.Errorf("lease lost; response cannot be committed")
	}
	current, err := snapshotCurrent(saveCtx, tx, r)
	if err != nil {
		return err
	}
	stale := !current
	if cancelRequested {
		status = "Cancelled"
	} else if pauseRequested && status == "Succeeded" {
		status = "Paused"
	}
	if disposition == llm.Retryable && r.AttemptCount < 2 && !cancelRequested && !pauseRequested {
		status = "RetryScheduled"
	}
	if r.Type == "memory-extraction" && status == "Succeeded" && !stale {
		proposal, proposalErr := s.CreateProposalTx(saveCtx, tx, r.ChapterID, r.Input.RevisionID, r.Input.StateVersionID, r.Input.Epoch, changes)
		if proposalErr != nil {
			status = "Failed"
			message = "记忆提议来源、基线或证据校验失败"
			disposition = llm.Permanent
		} else {
			output.ProposalID = proposal.ID
		}
	}
	raw := response.Raw
	if raw == nil {
		raw = []byte{}
	}
	if _, err = tx.Exec(saveCtx, "UPDATE wb_run_attempts SET status=$2,disposition=$3,raw=$4,usage=$5,finished_at=now() WHERE id=$1", attempt, status, disposition, raw, usage); err != nil {
		return err
	}
	out, _ := json.Marshal(output)
	if output.FormatRepaired {
		if err = eventTx(saveCtx, tx, r.ID, "format-repair", map[string]string{"strategy": "json-wrapper-and-trailing-comma", "validation": "closed-schema-and-evidence", "original": "preserved-in-attempt"}); err != nil {
			return err
		}
	}
	combinedUsage, cost, accountingErr := aggregateRunAccounting(saveCtx, tx, r.ProjectID, r.ID)
	if accountingErr != nil {
		return accountingErr
	}
	usage, _ = json.Marshal(combinedUsage)
	costStatus := "Unknown"
	if cost != nil {
		costStatus = "Estimated"
	}
	if _, err = tx.Exec(saveCtx, "UPDATE wb_runs SET status=$2,output=$3,error=$4,stale=$5,usage=$6,lease_owner='',lease_until=NULL,finished_at=CASE WHEN $2 IN ('Succeeded','Failed','Cancelled','NeedsReconciliation') THEN now() ELSE NULL END,next_attempt_at=now()+interval '3 seconds',cost=$7,cost_status=$8 WHERE id=$1", r.ID, status, out, message, stale, usage, cost, costStatus); err != nil {
		return err
	}
	if err = eventTx(saveCtx, tx, r.ID, "output", output); err != nil {
		return err
	}
	if err = eventTx(saveCtx, tx, r.ID, "status", map[string]any{"status": status, "stale": stale, "error": message}); err != nil {
		return err
	}
	if err = tx.Commit(saveCtx); err != nil {
		return err
	}
	if status == "Succeeded" && !stale && r.Input.UseCache && r.Type != "memory-extraction" {
		_ = s.PutRunCache(saveCtx, r.Input, output)
	}
	return nil
}

func (s *Service) completeCachedRun(ctx context.Context, r Run, output RunOutput) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var cancelRequested, pauseRequested bool
	if err = tx.QueryRow(ctx, "SELECT cancel_requested,pause_requested FROM wb_runs WHERE id=$1 AND status='Running' AND lease_owner=$2 AND lease_generation=$3 AND lease_until>now() FOR UPDATE", r.ID, r.LeaseOwner, r.LeaseGeneration).Scan(&cancelRequested, &pauseRequested); err != nil {
		return fmt.Errorf("cache result lease lost")
	}
	current, err := snapshotCurrent(ctx, tx, r)
	if err != nil {
		return err
	}
	if !current {
		return &Conflict{Message: "缓存输入已过期"}
	}
	output.Cached = true
	raw, _ := json.Marshal(output)
	status := "Succeeded"
	if cancelRequested {
		status = "Cancelled"
	} else if pauseRequested {
		status = "Paused"
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_runs SET status=$2,output=$3,cost=0,cost_status='Cached',usage='{}',lease_owner='',lease_until=NULL,finished_at=CASE WHEN $2='Paused' THEN NULL ELSE now() END WHERE id=$1", r.ID, status, raw); err != nil {
		return err
	}
	if err = eventTx(ctx, tx, r.ID, "output", output); err != nil {
		return err
	}
	if err = eventTx(ctx, tx, r.ID, "status", map[string]any{"status": status, "cached": true}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) failBeforeCall(ctx context.Context, r Run, cause error) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, "UPDATE wb_runs SET status='Failed',error=$4,stale=true,lease_owner='',lease_until=NULL,finished_at=now() WHERE id=$1 AND lease_owner=$2 AND lease_generation=$3", r.ID, r.LeaseOwner, r.LeaseGeneration, cause.Error())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("lease lost")
	}
	if err = eventTx(ctx, tx, r.ID, "status", map[string]string{"status": "Failed", "error": cause.Error()}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Worker(ctx context.Context) error {
	owner := job.NewID()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if err := s.AdvanceCopilot(ctx); err != nil {
			return err
		}
		if err := s.AdvanceAutomations(ctx); err != nil {
			return err
		}
		if err := s.AdvanceCandidates(ctx); err != nil {
			return err
		}
		if err := s.AdvanceMedia(ctx); err != nil {
			return err
		}
		if err := s.RecoverExpired(ctx); err != nil {
			return err
		}
		r, ok, err := s.ClaimRun(ctx, owner)
		if err != nil {
			return err
		}
		if ok {
			if err = s.ExecuteRun(ctx, r); err != nil {
				// A database error may leave an in-flight attempt. Expiry marks it unknown;
				// do not immediately repeat an external operation.
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil
				case <-timer.C:
				}
			}
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

var _ pgx.Row

// streamRun batches actual provider increments, never synthesized characters.
// First output is immediate; later output flushes every 75ms or 256 UTF-8 bytes.
// The timer also checks cancellation/lease while the provider waits silently.
func (s *Service) streamRun(ctx context.Context, r Run, provider llm.StreamingProvider) (llm.Response, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type increment struct {
		delta llm.Delta
		ack   chan error
	}
	type completion struct {
		response llm.Response
		err      error
	}
	increments := make(chan increment)
	completed := make(chan completion, 1)
	go func() {
		response, err := provider.Stream(streamCtx, r.Input.Request, func(delta llm.Delta) error {
			if delta.Text == "" {
				return nil
			}
			item := increment{delta: delta, ack: make(chan error, 1)}
			select {
			case increments <- item:
			case <-streamCtx.Done():
				return streamCtx.Err()
			}
			select {
			case err := <-item.ack:
				return err
			case <-streamCtx.Done():
				return streamCtx.Err()
			}
		})
		completed <- completion{response: response, err: err}
	}()
	ticker := time.NewTicker(75 * time.Millisecond)
	defer ticker.Stop()
	var received, pending strings.Builder
	first := true
	var persistenceErr error
	flush := func(flushCtx context.Context) error {
		if pending.Len() == 0 {
			return nil
		}
		if err := s.persistRunDelta(flushCtx, r, llm.Delta{Text: pending.String()}, received.String()); err != nil {
			return err
		}
		pending.Reset()
		return nil
	}
	for {
		select {
		case item := <-increments:
			if persistenceErr != nil {
				item.ack <- persistenceErr
				continue
			}
			received.WriteString(item.delta.Text)
			pending.WriteString(item.delta.Text)
			var err error
			if first || pending.Len() >= 256 {
				err = flush(ctx)
				first = false
			}
			item.ack <- err
			if err != nil {
				persistenceErr = err
				cancel()
			}
		case <-ticker.C:
			if persistenceErr != nil {
				continue
			}
			// Await the provider's cancellation result, then flush buffered
			// increments with the independent save context below.
			if ctx.Err() != nil {
				cancel()
				continue
			}
			var err error
			if pending.Len() > 0 {
				err = flush(ctx)
			} else {
				var valid bool
				err = s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_runs WHERE id=$1 AND status='Running' AND NOT cancel_requested AND lease_owner=$2 AND lease_generation=$3 AND lease_until>now())", r.ID, r.LeaseOwner, r.LeaseGeneration).Scan(&valid)
				if err == nil && !valid {
					err = fmt.Errorf("stream cancellation requested or lease unavailable")
				}
			}
			if err != nil {
				persistenceErr = err
				cancel()
			}
		case done := <-completed:
			// Even a network EOF/timeout must preserve received bytes that have
			// not reached the size/time threshold; use a separate save context.
			if persistenceErr == nil {
				saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
				persistenceErr = flush(saveCtx)
				saveCancel()
			}
			if done.response.Text == "" && received.Len() > 0 {
				done.response.Text = received.String()
			}
			if persistenceErr != nil {
				return done.response, &llm.Failure{Disposition: llm.NeedsReconciliation, Kind: "stream_consumer"}
			}
			return done.response, done.err
		}
	}
}

// persistRunDelta publishes only committed provider increments. SSE clients
// replay events by their durable ID; losing a subscription cannot cancel work.
func (s *Service) persistRunDelta(ctx context.Context, r Run, delta llm.Delta, partial string) error {
	if delta.Text == "" {
		return nil
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var cancelled bool
	if err = tx.QueryRow(ctx, "SELECT cancel_requested FROM wb_runs WHERE id=$1 AND status='Running' AND lease_owner=$2 AND lease_generation=$3 AND lease_until>now() FOR UPDATE", r.ID, r.LeaseOwner, r.LeaseGeneration).Scan(&cancelled); err != nil {
		return fmt.Errorf("stream lease unavailable")
	}
	if cancelled {
		return fmt.Errorf("stream cancellation requested")
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_runs SET output=jsonb_set(output,'{text}',to_jsonb($2::text),true) WHERE id=$1", r.ID, partial); err != nil {
		return err
	}
	if err = eventTx(ctx, tx, r.ID, "delta", delta); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
