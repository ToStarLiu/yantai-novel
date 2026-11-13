package workbench

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"novel/internal/llm"
)

func TestWorkflowDAGAndTypedPermissions(t *testing.T) {
	for _, definition := range []WorkflowDefinition{
		{Name: "循环", Version: 1, Steps: []WorkflowStep{{ID: "a", Type: "draft", DependsOn: []string{"b"}}, {ID: "b", Type: "review", DependsOn: []string{"a"}}}},
		{Name: "任意执行", Version: 1, Steps: []WorkflowStep{{ID: "shell", Type: "shell"}}},
		{Name: "悬空", Version: 1, Steps: []WorkflowStep{{ID: "a", Type: "draft", DependsOn: []string{"missing"}}}},
	} {
		if validateWorkflow(definition) == nil {
			t.Fatalf("invalid DAG accepted %#v", definition)
		}
	}
	s, _ := runtimeFixture(t)
	_, err := s.SaveAgent(context.Background(), AgentDefinition{Name: "错误权限", TaskType: "draft", MaxOutputTokens: 500, Tools: []string{"shell"}})
	if err == nil {
		t.Fatal("arbitrary tool execution accepted")
	}
}

func TestWorkflowRejectsMissingDefinitionsAndCorruptProductionDoesNotStopWorker(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	if _, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "missing-agent", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft", AgentID: "missing"}}}); err == nil {
		t.Fatal("missing agent accepted")
	}
	valid, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "valid-flow", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: valid.ID, ChapterIDs: []string{c.ID}, MaxChapters: 10})
	if err != nil {
		t.Fatal(err)
	}
	valid.Steps[0].PromptTemplateID = "removed-template"
	raw, _ := json.Marshal(valid)
	if _, err = s.DB.Exec(ctx, "UPDATE wb_productions SET workflow_snapshot=$2 WHERE id=$1", broken.ID, raw); err != nil {
		t.Fatal(err)
	}
	healthyDefinition, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "healthy-flow", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: healthyDefinition.ID, ChapterIDs: []string{c.ID}, MaxChapters: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal("one bad workflow stopped shared worker", err)
	}
	broken, err = s.GetProduction(ctx, broken.ID)
	if err != nil || broken.Status != "WaitingForUser" || broken.Error == "" {
		t.Fatalf("broken workflow not isolated %#v %v", broken, err)
	}
	healthy, err = s.GetProduction(ctx, healthy.ID)
	if err != nil || healthy.Steps[0].TaskRunID == "" {
		t.Fatalf("healthy workflow not advanced %#v %v", healthy, err)
	}
}

func TestProductionCheckpointHumanGateAndStableIntent(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	calls := 0
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		return llm.Response{Text: "独立候选正文", Raw: []byte("candidate")}, nil
	}}
	definition, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "持久流程", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft", HumanGate: true}, {ID: "conditional", Type: "reader", DependsOn: []string{"draft"}, Condition: "issues-found"}}})
	if err != nil {
		t.Fatal(err)
	}
	production, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: definition.ID, ChapterIDs: []string{c.ID}, MaxChapters: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	production, err = s.GetProduction(ctx, production.ID)
	if err != nil || production.Status != "WaitingForUser" {
		t.Fatalf("gate skipped %#v %v", production, err)
	}
	if _, err = s.ProductionAction(ctx, production.ID, "approve", production.Steps[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	production, err = s.GetProduction(ctx, production.ID)
	if err != nil {
		t.Fatal(err)
	}
	taskID := production.Steps[0].TaskRunID
	if taskID == "" {
		t.Fatal("task not submitted")
	}
	// Simulate a crash after SubmitRun committed but before its step pointer saved.
	if _, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET task_run_id='',status='Pending' WHERE id=$1", production.Steps[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	production, _ = s.GetProduction(ctx, production.ID)
	if production.Steps[0].TaskRunID != taskID {
		t.Fatal("checkpoint recovery submitted duplicate task")
	}
	var attempts int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_runs WHERE intent_id LIKE $1", "production:"+production.ID+":%").Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("duplicate intents %d %v", attempts, err)
	}
	if err = s.ExecuteRun(ctx, claim(t, s)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err = s.AdvanceAutomations(ctx); err != nil {
			t.Fatal(err)
		}
	}
	production, err = s.GetProduction(ctx, production.ID)
	if err != nil || production.Status != "Succeeded" || production.Steps[1].Status != "Skipped" || calls != 1 {
		t.Fatalf("production completion %#v calls=%d %v", production, calls, err)
	}
	ch, err := s.GetChapter(ctx, c.ID)
	if err != nil || ch.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("batch silently overwrote author manuscript")
	}
}

func TestProductionFiftyChapterBoundsAndPauseCancel(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	wf, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "批量", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: wf.ID, ChapterIDs: []string{c.ID}, MaxChapters: 51}); err == nil {
		t.Fatal("batch limit exceeded")
	}
	p, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: wf.ID, ChapterIDs: []string{c.ID}, MaxChapters: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProductionAction(ctx, p.ID, "pause", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetProduction(ctx, p.ID)
	if p.Steps[0].TaskRunID != "" {
		t.Fatal("paused production dispatched")
	}
	if _, err = s.ProductionAction(ctx, p.ID, "resume", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetProduction(ctx, p.ID)
	if !strings.HasPrefix(p.Steps[0].TaskRunID, "") || p.Steps[0].TaskRunID == "" {
		t.Fatal("resume did not dispatch")
	}
	if _, err = s.ProductionAction(ctx, p.ID, "cancel", ""); err != nil {
		t.Fatal(err)
	}
	task, err := s.GetRun(ctx, p.Steps[0].TaskRunID)
	if err != nil || task.Status != "Cancelled" {
		t.Fatalf("batch cancellation lost child %#v %v", task, err)
	}
}

func TestWorkflowUsesCandidateForReviewAndSelectedIssuesForRevision(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	calls := 0
	s.Provider = &llm.Fake{Handler: func(_ context.Context, request llm.Request) (llm.Response, error) {
		calls++
		text := "候选稿中的伤势突然消失。"
		if len(request.ResponseSchema) > 0 {
			text = `{"issues":[{"kind":"Continuity","severity":"Major","quote":"伤势突然消失","explanation":"缺少恢复来源","suggestedAction":"保留伤势"}]}`
		}
		return llm.Response{Text: text, Raw: []byte(text)}, nil
	}}
	wf, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "候选闭环", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}, {ID: "review", Type: "review", DependsOn: []string{"draft"}, InputSelection: "previous-candidate"}, {ID: "revision", Type: "revision", DependsOn: []string{"review"}, InputSelection: "previous-candidate", Condition: "issues-found"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: wf.ID, ChapterIDs: []string{c.ID}, MaxChapters: 10})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14; i++ {
		if err = s.AdvanceAutomations(ctx); err != nil {
			t.Fatal(err)
		}
		if run, ok, e := s.ClaimRun(ctx, "workflow-candidate-worker"); e != nil {
			t.Fatal(e)
		} else if ok {
			if err = s.ExecuteRun(ctx, run); err != nil {
				t.Fatal(err)
			}
		}
	}
	p, err = s.GetProduction(ctx, p.ID)
	if err != nil || p.Status != "Succeeded" || calls != 3 {
		t.Fatalf("candidate workflow failed %#v calls=%d %v", p, calls, err)
	}
	reviewed, err := s.GetRun(ctx, p.Steps[1].TaskRunID)
	if err != nil || reviewed.Input.CandidateRunID != p.Steps[0].TaskRunID || !strings.Contains(reviewed.Input.Source, "伤势突然消失") {
		t.Fatalf("review used author text instead of candidate %#v %v", reviewed, err)
	}
	revised, err := s.GetRun(ctx, p.Steps[2].TaskRunID)
	if err != nil || revised.Input.CandidateRunID != p.Steps[0].TaskRunID || len(revised.Input.SelectedIssues) != 1 {
		t.Fatalf("revision lost selected review issues %#v %v", revised, err)
	}
}

func TestProductionBudgetRejectsBeforeTaskBecomesVisible(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: "fake-v1", InputPricePerMillion: ptr(1), OutputPricePerMillion: ptr(1), Currency: "USD"}}
	workflow, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "有限批量", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft", MaxCost: ptr(0.000000001)}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: workflow.ID, ChapterIDs: []string{c.ID}, MaxChapters: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	p, err = s.GetProduction(ctx, p.ID)
	if err != nil || p.Status != "WaitingForUser" {
		t.Fatalf("budget did not stop production %#v %v", p, err)
	}
	var tasks int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_runs WHERE intent_id LIKE $1", "production:"+p.ID+":%").Scan(&tasks); err != nil || tasks != 0 {
		t.Fatalf("over-budget child became executable %d %v", tasks, err)
	}
	bad := WorkflowDefinition{Name: "NaN预算", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft", MaxCost: ptr(math.NaN())}}}
	if _, err = s.SaveWorkflow(ctx, bad); err == nil {
		t.Fatal("non-finite budget accepted")
	}
}

func TestProductionPauseGatesAnAlreadyQueuedChild(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	wf, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "排队暂停", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: wf.ID, ChapterIDs: []string{c.ID}, MaxChapters: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetProduction(ctx, p.ID)
	child := p.Steps[0].TaskRunID
	if _, err = s.ProductionAction(ctx, p.ID, "pause", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ClaimRun(ctx, "paused-production-worker"); err != nil || ok {
		t.Fatalf("paused production dispatched %v %v", ok, err)
	}
	task, _ := s.GetRun(ctx, child)
	if task.Status != "Paused" {
		t.Fatalf("child did not pause %s", task.Status)
	}
	if _, err = s.ProductionAction(ctx, p.ID, "resume", ""); err != nil {
		t.Fatal(err)
	}
	if task, ok, err := s.ClaimRun(ctx, "resumed-production-worker"); err != nil || !ok || task.ID != child {
		t.Fatalf("resume lost stable child %v %v", ok, err)
	}
}

func TestWorkflowEditingCreatesImmutableVersionAndDuplicateIsConflict(t *testing.T) {
	s, _ := runtimeFixture(t)
	ctx := context.Background()
	first, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "版本工作流", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.SaveWorkflow(ctx, first)
	if err != nil || next.ID == first.ID || next.Version != 2 {
		t.Fatalf("workflow replaced old version %#v %v", next, err)
	}
	first.ID = ""
	if _, err = s.SaveWorkflow(ctx, first); !IsConflict(err) {
		t.Fatalf("duplicate name version did not conflict %v", err)
	}
}
