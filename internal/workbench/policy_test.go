package workbench

import (
	"context"
	"encoding/json"
	"math"
	"novel/internal/job"
	"novel/internal/llm"
	"sync"
	"testing"
)

func ptr(v float64) *float64 { return &v }
func TestBudgetRejectsUnknownAndReservesPendingCosts(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	if _, e := s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: ptr(1), Currency: "USD", MaxOutputTokens: 500}); e != nil {
		t.Fatal(e)
	}
	_, e := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft", InputRevisionID: c.CurrentRevisionID})
	if e == nil {
		t.Fatal("unpriced call accepted within monetary budget")
	}
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: "fake-v1", InputPricePerMillion: ptr(1), OutputPricePerMillion: ptr(2), Currency: "USD"}}
	first := submit(t, s, c, "draft")
	input := int64(len(first.Input.Request.Messages[0].Content) + len(first.Input.Request.Messages[1].Content))
	estimated := estimateCost(first.Input.Policy, input, 500)
	if estimated == nil {
		t.Fatal("no estimate")
	}
	if _, e = s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: ptr(*estimated * 1.5), Currency: "USD", MaxOutputTokens: 500}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft", InputRevisionID: c.CurrentRevisionID}); e == nil {
		t.Fatal("pending reservation omitted")
	}
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Text: "候选", Raw: []byte("ok"), Usage: llm.Usage{InputTokens: 100, OutputTokens: 20, Reported: true}}, nil
	}}
	r := claim(t, s)
	if e = s.ExecuteRun(ctx, r); e != nil {
		t.Fatal(e)
	}
	done, _ := s.GetRun(ctx, r.ID)
	if done.Cost == nil || done.CostStatus != "Estimated" {
		t.Fatal("known price+usage not estimated")
	}
	sum, e := s.CostSummary(ctx, c.ProjectID)
	if e != nil || sum.KnownEstimatedCost <= 0 || sum.UnknownCalls != 0 {
		t.Fatalf("summary %+v %v", sum, e)
	}
}

func TestBudgetConcurrentSubmissionsReserveOnlyAffordableTasks(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: "fake-v1", InputPricePerMillion: ptr(1), OutputPricePerMillion: ptr(2), Currency: "USD"}}
	probe := submit(t, s, c, "draft")
	estimate := requestEstimate(probe.Input)
	if estimate == nil {
		t.Fatal("missing estimate")
	}
	if _, err := s.RunAction(ctx, probe.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: ptr(*estimate * 1.5), Currency: "USD", MaxOutputTokens: 500}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("concurrent budget reservation accepted %d tasks instead of 1", accepted)
	}
}

func TestAccountingUnknownAttemptDoesNotBecomeFreeAfterKnownRetry(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	s.Currency = "USD"
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: "fake-v1", InputPricePerMillion: ptr(1), OutputPricePerMillion: ptr(2), Currency: "USD"}}
	run := submit(t, s, c, "draft")
	// A prior unknown attempt is preserved independently from a later priced
	// response; this fixture does not automatically retry the unknown operation.
	if _, err := s.DB.Exec(ctx, "INSERT INTO wb_run_attempts(id,run_id,ordinal,status,request_snapshot,usage,finished_at) VALUES($1,$2,1,'NeedsReconciliation','{}','{}',now())", job.NewID(), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(ctx, "UPDATE wb_runs SET attempt_count=1 WHERE id=$1", run.ID); err != nil {
		t.Fatal(err)
	}
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Text: "候选", Raw: []byte("ok"), Usage: llm.Usage{Reported: true, InputTokens: 100, OutputTokens: 20}}, nil
	}}
	if err := s.ExecuteRun(ctx, claim(t, s)); err != nil {
		t.Fatal(err)
	}
	saved, err := s.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var usage llm.Usage
	if err = json.Unmarshal(saved.Usage, &usage); err != nil {
		t.Fatal(err)
	}
	if saved.Cost != nil || saved.CostStatus != "Unknown" || usage.Reported || usage.InputTokens != 100 {
		t.Fatalf("unknown accounting was hidden: cost=%v status=%s usage=%+v", saved.Cost, saved.CostStatus, usage)
	}
	summary, err := s.CostSummary(ctx, c.ProjectID)
	if err != nil || summary.UnknownCalls != 1 || summary.InputTokens != 100 || math.Abs(summary.KnownEstimatedCost-0.00014) > 1e-10 {
		t.Fatalf("lost partial ledger %+v %v", summary, err)
	}
	if _, err = s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: ptr(1), Currency: "USD", MaxOutputTokens: 500}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"}); err == nil {
		t.Fatal("unknown historical charge bypassed budget")
	}
}

func TestAccountingAggregatesAllAttemptsAndSeparatesCurrencies(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	s.Currency = "USD"
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: "fake-v1", InputPricePerMillion: ptr(1), OutputPricePerMillion: ptr(2), Currency: "USD"}}
	usd := submit(t, s, c, "draft")
	for i, u := range []llm.Usage{{Reported: true, InputTokens: 100, OutputTokens: 20}, {Reported: true, InputTokens: 80, OutputTokens: 10}} {
		raw, _ := json.Marshal(u)
		if _, err := s.DB.Exec(ctx, "INSERT INTO wb_run_attempts(id,run_id,ordinal,status,request_snapshot,usage,finished_at) VALUES($1,$2,$3,'Succeeded','{}',$4,now())", job.NewID(), usd.ID, i+1, raw); err != nil {
			t.Fatal(err)
		}
	}
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: "fake-v1", InputPricePerMillion: ptr(10), OutputPricePerMillion: ptr(20), Currency: "CNY"}}
	cny := submit(t, s, c, "draft")
	usage, _ := json.Marshal(llm.Usage{Reported: true, InputTokens: 200, OutputTokens: 30})
	if _, err := s.DB.Exec(ctx, "INSERT INTO wb_run_attempts(id,run_id,ordinal,status,request_snapshot,usage,finished_at) VALUES($1,$2,1,'Succeeded','{}',$3,now())", job.NewID(), cny.ID, usage); err != nil {
		t.Fatal(err)
	}
	summary, err := s.CostSummary(ctx, c.ProjectID)
	if err != nil || summary.InputTokens != 380 || summary.OutputTokens != 60 || summary.UnknownCalls != 0 || len(summary.Amounts) != 2 {
		t.Fatalf("ledger %+v %v", summary, err)
	}
	if math.Abs(summary.KnownEstimatedCost-0.00024) > 1e-10 {
		t.Fatalf("cross-currency addition %.8f", summary.KnownEstimatedCost)
	}
	if _, err = s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: ptr(1), Currency: "USD", MaxOutputTokens: 500}); err == nil {
		t.Fatal("mixed-currency budget accepted")
	}
	if estimateCost(ModelPolicy{InputPricePerMillion: ptr(math.NaN()), OutputPricePerMillion: ptr(1), Currency: "USD"}, 1, 1) != nil {
		t.Fatal("NaN price produced money")
	}
}
func TestTaskModelRoutingAndOutputCap(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	s.ModelPolicies = map[string]ModelPolicy{"review": {Model: "review-model"}}
	if _, e := s.SavePreferences(ctx, c.ProjectID, Preferences{MaxOutputTokens: 900}); e != nil {
		t.Fatal(e)
	}
	r := submit(t, s, c, "review")
	if r.Input.Request.Model != "review-model" || r.Input.Request.MaxOutputTokens != 900 {
		t.Fatalf("route %+v", r.Input.Request)
	}
	claimed := claim(t, s)
	if claimed.ID != r.ID {
		t.Fatal("routed model not claimable")
	}
}
