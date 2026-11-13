package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/jackc/pgx/v5"
	"novel/internal/llm"
)

type ModelPolicy struct {
	Model                 string   `json:"model"`
	InputPricePerMillion  *float64 `json:"inputPricePerMillion"`
	OutputPricePerMillion *float64 `json:"outputPricePerMillion"`
	Currency              string   `json:"currency"`
}
type Preferences struct {
	MaxCost         *float64 `json:"maxCost"`
	Currency        string   `json:"currency"`
	MaxOutputTokens int      `json:"maxOutputTokens"`
}
type CurrencyAmount struct {
	Currency           string  `json:"currency"`
	KnownEstimatedCost float64 `json:"knownEstimatedCost"`
	UnknownCalls       int     `json:"unknownCalls"`
}
type CostSummary struct {
	KnownEstimatedCost float64          `json:"knownEstimatedCost"`
	UnknownCalls       int              `json:"unknownCalls"`
	InputTokens        int64            `json:"inputTokens"`
	OutputTokens       int64            `json:"outputTokens"`
	Currency           string           `json:"currency"`
	Amounts            []CurrencyAmount `json:"amounts"`
	Preferences        Preferences      `json:"preferences"`
}

func (s *Service) policy(kind string) ModelPolicy {
	p, ok := s.ModelPolicies[kind]
	if !ok {
		p = s.ModelPolicies["default"]
	}
	if p.Model == "" {
		p.Model = s.Model
	}
	return p
}
func (s *Service) ModelRoutes() map[string]ModelPolicy {
	out := map[string]ModelPolicy{}
	for _, kind := range []string{"draft", "review", "revision", "memory-extraction", "planning", "rewrite", "handoff"} {
		out[kind] = s.policy(kind)
	}
	return out
}
func (s *Service) Preferences(ctx context.Context, pid string) (Preferences, error) {
	p := Preferences{MaxOutputTokens: s.MaxOutputTokens, Currency: s.Currency}
	if p.MaxOutputTokens < 1 {
		p.MaxOutputTokens = 3000
	}
	err := s.DB.QueryRow(ctx, "SELECT max_cost,currency,max_output_tokens FROM wb_preferences WHERE project_id=$1", pid).Scan(&p.MaxCost, &p.Currency, &p.MaxOutputTokens)
	if err == pgx.ErrNoRows {
		return p, nil
	}
	return p, err
}
func (s *Service) SavePreferences(ctx context.Context, pid string, p Preferences) (Preferences, error) {
	if p.MaxOutputTokens < 256 || p.MaxOutputTokens > 12000 {
		return p, fmt.Errorf("输出上限必须介于256至12000")
	}
	if p.MaxCost != nil && (math.IsNaN(*p.MaxCost) || math.IsInf(*p.MaxCost, 0) || *p.MaxCost <= 0 || p.Currency == "") {
		return p, fmt.Errorf("预算必须是正数并指定币种")
	}
	if len(p.Currency) > 12 {
		return p, fmt.Errorf("币种过长")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR UPDATE", pid).Scan(&id); err != nil {
		return p, noRows(err)
	}
	if p.MaxCost != nil {
		var mismatch bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wb_run_attempts a JOIN wb_runs r ON r.id=a.run_id WHERE r.project_id=$1 AND COALESCE(r.input->'priceSnapshot'->>'currency','')<>$2)`, pid, p.Currency).Scan(&mismatch); err != nil {
			return p, err
		}
		if mismatch {
			return p, fmt.Errorf("历史调用存在其他币种或未知币种，不能用新币种合并作品预算")
		}
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wb_copilot_attempts WHERE project_id=$1 AND COALESCE(policy->>'currency','')<>$2)`, pid, p.Currency).Scan(&mismatch); err != nil {
			return p, err
		}
		if mismatch {
			return p, fmt.Errorf("历史对话调用存在其他币种或未知币种，不能合并作品预算")
		}
	}
	_, err = tx.Exec(ctx, "INSERT INTO wb_preferences(project_id,max_cost,currency,max_output_tokens) VALUES($1,$2,$3,$4) ON CONFLICT(project_id) DO UPDATE SET max_cost=$2,currency=$3,max_output_tokens=$4,updated_at=now()", pid, p.MaxCost, p.Currency, p.MaxOutputTokens)
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

type ledgerAttempt struct {
	RunID, RunStatus, AttemptStatus string
	Policy                          ModelPolicy
	Usage                           llm.Usage
}

func costLedger(ctx context.Context, db queryer, pid, runID string) ([]ledgerAttempt, error) {
	rows, err := db.Query(ctx, `SELECT a.run_id,r.status,a.status,r.input,a.usage FROM wb_run_attempts a JOIN wb_runs r ON r.id=a.run_id WHERE r.project_id=$1 AND ($2='' OR r.id=$2) ORDER BY a.started_at,a.id`, pid, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []ledgerAttempt{}
	for rows.Next() {
		var entry ledgerAttempt
		var input, usage []byte
		if err = rows.Scan(&entry.RunID, &entry.RunStatus, &entry.AttemptStatus, &input, &usage); err != nil {
			return nil, err
		}
		var snapshot RunInput
		if err = json.Unmarshal(input, &snapshot); err != nil {
			return nil, err
		}
		entry.Policy = snapshot.Policy
		if len(usage) > 0 && string(usage) != "null" {
			if err = json.Unmarshal(usage, &entry.Usage); err != nil {
				return nil, err
			}
		}
		entries = append(entries, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if runID == "" {
		extra, e := db.Query(ctx, `SELECT a.turn_id,t.status,a.status,a.policy,a.usage FROM wb_copilot_attempts a JOIN wb_copilot_turns t ON t.id=a.turn_id WHERE a.project_id=$1 ORDER BY a.started_at,a.id`, pid)
		if e != nil {
			return nil, e
		}
		defer extra.Close()
		for extra.Next() {
			var entry ledgerAttempt
			var policy, usage []byte
			if e = extra.Scan(&entry.RunID, &entry.RunStatus, &entry.AttemptStatus, &policy, &usage); e != nil {
				return nil, e
			}
			if e = json.Unmarshal(policy, &entry.Policy); e != nil {
				return nil, e
			}
			if e = json.Unmarshal(usage, &entry.Usage); e != nil {
				return nil, e
			}
			entries = append(entries, entry)
		}
		if e = extra.Err(); e != nil {
			return nil, e
		}
	}
	return entries, nil
}
func (s *Service) CostSummary(ctx context.Context, pid string) (CostSummary, error) {
	out := CostSummary{Amounts: []CurrencyAmount{}}
	prefs, err := s.Preferences(ctx, pid)
	if err != nil {
		return out, err
	}
	out.Preferences = prefs
	out.Currency = prefs.Currency
	ledger, err := costLedger(ctx, s.DB, pid, "")
	if err != nil {
		return out, err
	}
	groups := map[string]*CurrencyAmount{}
	for _, entry := range ledger {
		amount, ok := groups[entry.Policy.Currency]
		if !ok {
			amount = &CurrencyAmount{Currency: entry.Policy.Currency}
			groups[entry.Policy.Currency] = amount
		}
		if entry.Usage.Reported {
			out.InputTokens += entry.Usage.InputTokens
			out.OutputTokens += entry.Usage.OutputTokens
		}
		if cost := costForUsage(entry.Policy, entry.Usage); cost != nil {
			amount.KnownEstimatedCost += *cost
		} else {
			amount.UnknownCalls++
			out.UnknownCalls++
		}
	}
	for _, amount := range groups {
		out.Amounts = append(out.Amounts, *amount)
		if amount.Currency == out.Currency {
			out.KnownEstimatedCost = amount.KnownEstimatedCost
		}
	}
	sort.Slice(out.Amounts, func(i, j int) bool { return out.Amounts[i].Currency < out.Amounts[j].Currency })
	return out, nil
}
func estimateCost(p ModelPolicy, inputTokens, outputTokens int64) *float64 {
	if p.InputPricePerMillion == nil || p.OutputPricePerMillion == nil || p.Currency == "" || inputTokens < 0 || outputTokens < 0 {
		return nil
	}
	for _, price := range []float64{*p.InputPricePerMillion, *p.OutputPricePerMillion} {
		if price < 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			return nil
		}
	}
	cost := float64(inputTokens)**p.InputPricePerMillion/1e6 + float64(outputTokens)**p.OutputPricePerMillion/1e6
	if math.IsInf(cost, 0) || math.IsNaN(cost) {
		return nil
	}
	return &cost
}
func costForUsage(p ModelPolicy, u llm.Usage) *float64 {
	if !u.Reported {
		return nil
	}
	return estimateCost(p, u.InputTokens, u.OutputTokens)
}
func requestEstimate(input RunInput) *float64 {
	var bytes int64
	for _, message := range input.Request.Messages {
		bytes += int64(len(message.Content))
	}
	return estimateCost(input.Policy, bytes, int64(input.Request.MaxOutputTokens))
}

// aggregateRunAccounting retains every attempt, including failures and
// cancellations. A later priced response cannot make an earlier unknown free.
func aggregateRunAccounting(ctx context.Context, db queryer, pid, runID string) (llm.Usage, *float64, error) {
	entries, err := costLedger(ctx, db, pid, runID)
	if err != nil {
		return llm.Usage{}, nil, err
	}
	usage := llm.Usage{Reported: len(entries) > 0}
	sum := 0.0
	known := len(entries) > 0
	currency := ""
	for _, entry := range entries {
		if entry.Usage.Reported {
			usage.InputTokens += entry.Usage.InputTokens
			usage.OutputTokens += entry.Usage.OutputTokens
			usage.CachedInputTokens += entry.Usage.CachedInputTokens
		} else {
			usage.Reported = false
		}
		if currency == "" {
			currency = entry.Policy.Currency
		} else if currency != entry.Policy.Currency {
			known = false
		}
		cost := costForUsage(entry.Policy, entry.Usage)
		if cost == nil {
			known = false
		} else {
			sum += *cost
		}
	}
	if !known {
		return usage, nil, nil
	}
	return usage, &sum, nil
}
func (s *Service) checkBudgetTx(ctx context.Context, tx pgx.Tx, pid, excludeID string, input RunInput) error {
	estimated := requestEstimate(input)
	if s.MaxJobCost != nil && (estimated == nil || *estimated > *s.MaxJobCost) {
		return fmt.Errorf("本次费用估算未知或超过单任务预算，未发送模型请求")
	}
	var budget *float64
	var currency string
	err := tx.QueryRow(ctx, "SELECT max_cost,currency FROM wb_preferences WHERE project_id=$1", pid).Scan(&budget, &currency)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if budget == nil {
		return nil
	}
	if estimated == nil || currency != input.Policy.Currency {
		return fmt.Errorf("作品设有预算，需先配置同币种模型价格")
	}
	ledger, err := costLedger(ctx, tx, pid, "")
	if err != nil {
		return err
	}
	spent := 0.0
	for _, entry := range ledger {
		// Active requests are covered by reservations below. Once a run leaves
		// Running, missing usage remains unknown, including interrupted attempts.
		if entry.AttemptStatus == "Running" && entry.RunStatus == "Running" {
			continue
		}
		if entry.Policy.Currency != currency {
			return fmt.Errorf("历史调用币种与当前预算不同，不能混合结算")
		}
		cost := costForUsage(entry.Policy, entry.Usage)
		if cost == nil {
			return fmt.Errorf("存在费用未知的历史调用，请先核对预算")
		}
		spent += *cost
	}
	rows, err := tx.Query(ctx, "SELECT input FROM wb_runs WHERE project_id=$1 AND id<>$2 AND status IN ('Pending','Running','RetryScheduled')", pid, excludeID)
	if err != nil {
		return err
	}
	defer rows.Close()
	reserved := 0.0
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		var queued RunInput
		if err = json.Unmarshal(raw, &queued); err != nil {
			return err
		}
		cost := requestEstimate(queued)
		if cost == nil || queued.Policy.Currency != currency {
			return fmt.Errorf("存在未定价或不同币种的排队任务")
		}
		reserved += *cost
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	copilotRows, err := tx.Query(ctx, `SELECT a.reserved_cost,a.policy FROM wb_copilot_attempts a JOIN wb_copilot_turns t ON t.id=a.turn_id WHERE a.project_id=$1 AND a.status='Running' AND t.status='Running'`, pid)
	if err != nil {
		return err
	}
	defer copilotRows.Close()
	for copilotRows.Next() {
		var cost *float64
		var raw []byte
		if err = copilotRows.Scan(&cost, &raw); err != nil {
			return err
		}
		var policy ModelPolicy
		if err = json.Unmarshal(raw, &policy); err != nil {
			return err
		}
		if cost == nil || policy.Currency != currency {
			return fmt.Errorf("存在未定价或不同币种的执行中对话")
		}
		reserved += *cost
	}
	if err = copilotRows.Err(); err != nil {
		return err
	}
	if spent+reserved+*estimated > *budget {
		return fmt.Errorf("作品预算不足，未发送模型请求")
	}
	return nil
}
