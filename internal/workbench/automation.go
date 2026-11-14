package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"novel/internal/job"
)

type WorkflowStep struct {
	ID               string        `json:"id"`
	Type             string        `json:"type"`
	PromptTemplateID string        `json:"promptTemplateId"`
	AgentID          string        `json:"agentId"`
	DependsOn        []string      `json:"dependsOn"`
	Condition        string        `json:"condition"`
	HumanGate        bool          `json:"humanGate"`
	MaxCost          *float64      `json:"maxCost"`
	InputSelection   string        `json:"inputSelection"`
	MCPCalls         []MCPToolCall `json:"mcpCalls,omitempty"`
}
type WorkflowDefinition struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Version int            `json:"version"`
	Steps   []WorkflowStep `json:"steps"`
}
type ProductionRequest struct {
	IgnoreMemory bool     `json:"ignoreMemory,omitempty"`
	WorkflowID   string   `json:"workflowId"`
	ChapterIDs   []string `json:"chapterIds"`
	StartOrder   int      `json:"startOrder"`
	EndOrder     int      `json:"endOrder"`
	MaxChapters  int      `json:"maxChapters"`
	MaxCost      *float64 `json:"maxCost"`
	Currency     string   `json:"currency"`
	ManualGates  bool     `json:"manualGates"`
}
type ProductionStep struct {
	ID        string `json:"id"`
	ChapterID string `json:"chapterId"`
	StepID    string `json:"stepId"`
	Order     int    `json:"order"`
	Status    string `json:"status"`
	TaskRunID string `json:"taskRunId"`
	Approved  bool   `json:"approved"`
	Error     string `json:"error"`
}
type ProductionRun struct {
	OwnerID      string             `json:"ownerId,omitempty"`
	IgnoreMemory bool               `json:"ignoreMemory,omitempty"`
	ID           string             `json:"id"`
	ProjectID    string             `json:"projectId"`
	Workflow     WorkflowDefinition `json:"workflow"`
	ChapterIDs   []string           `json:"chapterIds"`
	MaxCost      *float64           `json:"maxCost"`
	Currency     string             `json:"currency"`
	ManualGates  bool               `json:"manualGates"`
	Status       string             `json:"status"`
	Error        string             `json:"error"`
	CreatedAt    time.Time          `json:"createdAt"`
	Steps        []ProductionStep   `json:"steps"`
}

func advancedTask(kind string) bool {
	switch kind {
	case "planning", "draft", "review", "revision", "memory-extraction", "analysis", "translation", "reader", "developmental", "voice", "publisher", "rewrite", "handoff":
		return true
	}
	return false
}
func validateWorkflow(in WorkflowDefinition) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || in.Version < 1 || len(in.Steps) == 0 || len(in.Steps) > 30 {
		return fmt.Errorf("工作流需要名称、正版本与1至30个步骤")
	}
	ids := map[string]bool{}
	for _, step := range in.Steps {
		if step.ID == "" || len(step.ID) > 100 || strings.ContainsAny(step.ID, "\r\n:") || ids[step.ID] || !advancedTask(step.Type) {
			return fmt.Errorf("步骤ID重复或任务类型无效")
		}
		ids[step.ID] = true
		if step.Condition != "" && step.Condition != "always" && step.Condition != "issues-found" && step.Condition != "no-issues" {
			return fmt.Errorf("条件必须为always/issues-found/no-issues")
		}
		if step.InputSelection != "" && step.InputSelection != "current" && step.InputSelection != "previous-candidate" {
			return fmt.Errorf("步骤输入只能为current/previous-candidate")
		}
		if step.MaxCost != nil && (*step.MaxCost <= 0 || math.IsNaN(*step.MaxCost) || math.IsInf(*step.MaxCost, 0)) {
			return fmt.Errorf("步骤预算必须为正数")
		}
		if len(step.MCPCalls) > 8 || len(step.MCPCalls) > 0 && step.AgentID == "" {
			return fmt.Errorf("MCP步骤需要Agent且最多8次调用")
		}
	}
	colors := map[string]int{}
	byID := map[string]WorkflowStep{}
	for _, step := range in.Steps {
		byID[step.ID] = step
	}
	var visit func(string) error
	visit = func(id string) error {
		if colors[id] == 1 {
			return fmt.Errorf("工作流依赖存在循环")
		}
		if colors[id] == 2 {
			return nil
		}
		colors[id] = 1
		seen := map[string]bool{}
		for _, dep := range byID[id].DependsOn {
			if !ids[dep] || dep == id || seen[dep] {
				return fmt.Errorf("步骤依赖无效")
			}
			seen[dep] = true
			if err := visit(dep); err != nil {
				return err
			}
		}
		colors[id] = 2
		return nil
	}
	for id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) SaveWorkflow(ctx context.Context, in WorkflowDefinition) (WorkflowDefinition, error) {
	if err := validateWorkflow(in); err != nil {
		return in, err
	}
	for _, step := range in.Steps {
		if step.PromptTemplateID != "" {
			if _, err := s.ResolvePrompt(ctx, step.Type, step.PromptTemplateID); err != nil {
				return in, err
			}
		}
		if step.AgentID != "" {
			agent, err := s.GetAgent(ctx, step.AgentID)
			if err != nil {
				return in, err
			}
			if agent.TaskType != step.Type {
				return in, fmt.Errorf("Agent任务类型与工作流步骤不符")
			}
			permissions := map[string]bool{}
			for _, tool := range agent.Tools {
				permissions[tool] = true
			}
			for _, call := range step.MCPCalls {
				if !permissions["mcp:"+call.ConfigID+":"+call.Name] {
					return in, fmt.Errorf("工作流Agent没有MCP调用权限")
				}
			}
		}
	}
	if in.ID != "" {
		var oldVersion int
		err := s.DB.QueryRow(ctx, "SELECT version FROM wb_workflows WHERE id=$1", in.ID).Scan(&oldVersion)
		if err != nil {
			return in, noRows(err)
		}
		if in.Version <= oldVersion {
			in.Version = oldVersion + 1
		}
	}
	in.ID = job.NewID()
	raw, _ := json.Marshal(in)
	_, err := s.DB.Exec(ctx, "INSERT INTO wb_workflows(id,name,version,definition) VALUES($1,$2,$3,$4)", in.ID, in.Name, in.Version, raw)
	if dbErr, ok := err.(*pgconn.PgError); ok && dbErr.Code == "23505" {
		return in, &Conflict{Message: "工作流名称与版本已存在，请增加版本号"}
	}
	return in, err
}
func (s *Service) ListWorkflows(ctx context.Context) ([]WorkflowDefinition, error) {
	rows, err := s.DB.Query(ctx, "SELECT definition FROM wb_workflows ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowDefinition{}
	for rows.Next() {
		var raw []byte
		var d WorkflowDefinition
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Service) StartProduction(ctx context.Context, projectID string, in ProductionRequest) (ProductionRun, error) {
	var raw []byte
	if err := s.DB.QueryRow(ctx, "SELECT definition FROM wb_workflows WHERE id=$1", in.WorkflowID).Scan(&raw); err != nil {
		return ProductionRun{}, noRows(err)
	}
	var definition WorkflowDefinition
	if err := json.Unmarshal(raw, &definition); err != nil {
		return ProductionRun{}, err
	}
	if err := validateWorkflow(definition); err != nil {
		return ProductionRun{}, err
	}
	detail, err := s.GetProject(ctx, projectID)
	if err != nil {
		return ProductionRun{}, err
	}
	if in.MaxChapters == 0 {
		in.MaxChapters = 50
	}
	if in.MaxChapters < 1 || in.MaxChapters > 50 {
		return ProductionRun{}, fmt.Errorf("批量章节上限应为1至50")
	}
	if in.MaxCost != nil && (*in.MaxCost <= 0 || in.Currency == "" || math.IsNaN(*in.MaxCost) || math.IsInf(*in.MaxCost, 0)) {
		return ProductionRun{}, fmt.Errorf("批量预算需要正金额和币种")
	}
	chapters := []string{}
	available := map[string]bool{}
	for _, chapter := range detail.Chapters {
		available[chapter.ID] = true
		if len(in.ChapterIDs) == 0 && (in.StartOrder == 0 || chapter.Order >= in.StartOrder) && (in.EndOrder == 0 || chapter.Order <= in.EndOrder) {
			chapters = append(chapters, chapter.ID)
		}
	}
	if len(in.ChapterIDs) > 0 {
		chapters = in.ChapterIDs
	}
	if len(chapters) == 0 || len(chapters) > in.MaxChapters {
		return ProductionRun{}, fmt.Errorf("批量范围为空或超过章数上限")
	}
	seen := map[string]bool{}
	for _, id := range chapters {
		if !available[id] || seen[id] {
			return ProductionRun{}, fmt.Errorf("章节不属于作品或重复")
		}
		seen[id] = true
	}
	batch, err := s.prepareAuthorBatch(ctx, projectID, in.IgnoreMemory)
	if err != nil {
		return ProductionRun{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ProductionRun{}, err
	}
	defer tx.Rollback(ctx)
	id := job.NewID()
	chapterJSON, _ := json.Marshal(chapters)
	if _, err = tx.Exec(ctx, "INSERT INTO wb_productions(id,project_id,workflow_snapshot,chapter_ids,max_cost,currency,manual_gates,status) VALUES($1,$2,$3,$4,$5,$6,$7,'Running')", id, projectID, raw, chapterJSON, in.MaxCost, in.Currency, in.ManualGates); err != nil {
		return ProductionRun{}, err
	}
	if batch != nil {
		valid, e := authorMemoryBaselineCurrent(ctx, tx, batch.Baseline)
		if e != nil {
			return ProductionRun{}, e
		}
		if !valid {
			return ProductionRun{}, &Conflict{Message: "作者记忆已变化，请重新启动"}
		}
		if e = saveAuthorBatchTx(ctx, tx, "production", id, batch); e != nil {
			return ProductionRun{}, e
		}
	}
	order := 0
	for _, chapter := range chapters {
		for _, step := range definition.Steps {
			order++
			if _, err = tx.Exec(ctx, "INSERT INTO wb_production_steps(id,production_id,chapter_id,step_id,sort_order,status) VALUES($1,$2,$3,$4,$5,'Pending')", job.NewID(), id, chapter, step.ID, order); err != nil {
				return ProductionRun{}, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return ProductionRun{}, err
	}
	return s.GetProduction(ctx, id)
}
func (s *Service) GetProduction(ctx context.Context, id string) (ProductionRun, error) {
	var out ProductionRun
	var definition, chapters []byte
	err := s.DB.QueryRow(ctx, "SELECT id,project_id,workflow_snapshot,chapter_ids,max_cost,currency,manual_gates,status,error,created_at FROM wb_productions WHERE id=$1", id).Scan(&out.ID, &out.ProjectID, &definition, &chapters, &out.MaxCost, &out.Currency, &out.ManualGates, &out.Status, &out.Error, &out.CreatedAt)
	if err != nil {
		return out, noRows(err)
	}
	if err = json.Unmarshal(definition, &out.Workflow); err != nil {
		return out, err
	}
	if err = json.Unmarshal(chapters, &out.ChapterIDs); err != nil {
		return out, err
	}
	var batchOwner string
	var batchIgnore bool
	batchErr := s.DB.QueryRow(ctx, "SELECT owner_id,ignore_memory FROM wb_production_author_contexts WHERE production_id=$1", id).Scan(&batchOwner, &batchIgnore)
	if batchErr != nil && batchErr != pgx.ErrNoRows {
		return out, batchErr
	}
	out.OwnerID, out.IgnoreMemory = batchOwner, batchIgnore
	out.Steps = []ProductionStep{}
	rows, err := s.DB.Query(ctx, "SELECT id,chapter_id,step_id,sort_order,status,task_run_id,approved,error FROM wb_production_steps WHERE production_id=$1 ORDER BY sort_order", id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var step ProductionStep
		if err = rows.Scan(&step.ID, &step.ChapterID, &step.StepID, &step.Order, &step.Status, &step.TaskRunID, &step.Approved, &step.Error); err != nil {
			return out, err
		}
		out.Steps = append(out.Steps, step)
	}
	return out, rows.Err()
}
func (s *Service) ListProductions(ctx context.Context, pid string) ([]ProductionRun, error) {
	rows, err := s.DB.Query(ctx, "SELECT id FROM wb_productions WHERE project_id=$1 ORDER BY created_at DESC LIMIT 100", pid)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []ProductionRun{}
	for _, id := range ids {
		r, err := s.GetProduction(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *Service) ProductionAction(ctx context.Context, id, action, stepID string) (ProductionRun, error) {
	p, err := s.GetProduction(ctx, id)
	if err != nil {
		return p, err
	}
	if p.OwnerID != "" {
		if actor, ok := PrincipalFromContext(ctx); ok && actor.ID != p.OwnerID {
			return p, ErrForbidden
		}
		if action == "resume" || action == "approve" {
			if _, _, checkErr := s.authorBatchExecutionContext(ctx, "production", id, p.ProjectID); checkErr != nil {
				return p, checkErr
			}
		}
	}
	switch action {
	case "pause":
		if p.Status != "Running" {
			return p, &Conflict{Message: "仅运行中的批量可暂停"}
		}
		_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='Paused' WHERE id=$1", id)
		if err == nil {
			for _, step := range p.Steps {
				if step.TaskRunID != "" {
					run, e := s.GetRun(ctx, step.TaskRunID)
					if e == nil && (run.Status == "Pending" || run.Status == "Running" || run.Status == "RetryScheduled") {
						_, _ = s.RunAction(ctx, run.ID, "pause")
					}
				}
			}
		}
	case "resume":
		if p.Status != "Paused" && p.Status != "WaitingForUser" && p.Status != "Failed" {
			return p, &Conflict{Message: "当前批量不可恢复"}
		}
		_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='Running',error='' WHERE id=$1", id)
		if err == nil {
			for _, step := range p.Steps {
				if step.TaskRunID != "" {
					run, e := s.GetRun(ctx, step.TaskRunID)
					if e == nil && run.Status == "Paused" {
						if _, e = s.RunAction(ctx, run.ID, "resume"); e != nil {
							_, _ = s.DB.Exec(ctx, "UPDATE wb_productions SET status='WaitingForUser',error=$2 WHERE id=$1", id, e.Error())
							return s.GetProduction(ctx, id)
						}
					}
				}
			}
		}
	case "cancel":
		_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='Cancelled',finished_at=now() WHERE id=$1 AND status NOT IN ('Succeeded','Cancelled')", id)
		if err == nil {
			for _, step := range p.Steps {
				if step.TaskRunID != "" && (step.Status == "Running" || step.Status == "Pending") {
					_, _ = s.RunAction(ctx, step.TaskRunID, "cancel")
				}
			}
		}
	case "approve":
		var matched bool
		for _, step := range p.Steps {
			if step.ID == stepID && step.Status == "WaitingForUser" {
				matched = true
			}
		}
		if !matched {
			return p, fmt.Errorf("待确认步骤不存在")
		}
		_, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET approved=true,status='Pending' WHERE id=$1 AND production_id=$2", stepID, id)
		if err == nil {
			_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='Running',error='' WHERE id=$1", id)
		}
	default:
		return p, fmt.Errorf("未知批量操作")
	}
	if err != nil {
		return p, err
	}
	return s.GetProduction(ctx, id)
}

func (s *Service) AdvanceAutomations(ctx context.Context) error {
	// A session-level lock spans the separate durable SubmitRun transaction. If
	// this process stops after submission, stable intent finds the same task.
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtext(current_database()||current_schema()||'automation-advance'))").Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext(current_database()||current_schema()||'automation-advance'))")
	rows, err := conn.Query(ctx, "SELECT id FROM wb_productions WHERE status='Running' ORDER BY created_at LIMIT 20")
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
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.advanceProduction(ctx, id); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// A malformed or no longer available template/provider belongs to this
			// production. It must not terminate the shared worker for every project.
			if _, saveErr := s.DB.Exec(ctx, "UPDATE wb_productions SET status='WaitingForUser',error=$2 WHERE id=$1 AND status='Running'", id, err.Error()); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}
func (s *Service) advanceProduction(ctx context.Context, id string) error {
	p, err := s.GetProduction(ctx, id)
	if err != nil {
		return err
	}
	executionCtx, ignoreMemory, ownerErr := s.authorBatchExecutionContext(ctx, "production", id, p.ProjectID)
	if ownerErr != nil {
		_, saveErr := s.DB.Exec(ctx, "UPDATE wb_productions SET status='WaitingForUser',error=$2 WHERE id=$1", id, "作者上下文或作品权限已变化，请重新核对批量任务")
		return saveErr
	}
	defs := map[string]WorkflowStep{}
	for _, step := range p.Workflow.Steps {
		defs[step.ID] = step
	}
	complete := true
	for _, step := range p.Steps {
		if step.Status == "Succeeded" || step.Status == "Skipped" {
			continue
		}
		complete = false
		definition := defs[step.StepID]
		if step.TaskRunID != "" {
			run, err := s.GetRun(ctx, step.TaskRunID)
			if err != nil {
				return err
			}
			switch run.Status {
			case "Succeeded":
				if run.Stale {
					_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='WaitingForUser',error='批量步骤输入已过期，请复核候选' WHERE id=$1", id)
					return err
				}
				_, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET status='Succeeded' WHERE id=$1", step.ID)
				return err
			case "Failed", "Cancelled", "NeedsReconciliation":
				_, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET status=$2,error=$3 WHERE id=$1", step.ID, run.Status, run.Error)
				if err != nil {
					return err
				}
				_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='WaitingForUser',error=$2 WHERE id=$1", id, "步骤需要人工处理："+run.Status)
				return err
			default:
				return nil
			}
		}
		// Chapters are processed in explicit order; steps within a chapter follow
		// the declared DAG, including conditions tied to completed review output.
		chapterReady := true
		for _, prior := range p.Steps {
			if prior.Order >= step.Order {
				break
			}
			if prior.ChapterID != step.ChapterID && prior.Status != "Succeeded" && prior.Status != "Skipped" {
				chapterReady = false
			}
		}
		if !chapterReady {
			return nil
		}
		dependencies := []Run{}
		ready := true
		for _, dep := range definition.DependsOn {
			found := false
			for _, candidate := range p.Steps {
				if candidate.ChapterID == step.ChapterID && candidate.StepID == dep {
					found = true
					if candidate.Status != "Succeeded" && candidate.Status != "Skipped" {
						ready = false
					} else if candidate.TaskRunID != "" {
						run, e := s.GetRun(ctx, candidate.TaskRunID)
						if e != nil {
							return e
						}
						dependencies = append(dependencies, run)
					}
				}
			}
			if !found {
				ready = false
			}
		}
		if !ready {
			continue
		}
		hasIssues := false
		for _, dep := range dependencies {
			if len(dep.Output.Issues) > 0 {
				hasIssues = true
			}
		}
		if definition.Condition == "issues-found" && !hasIssues || definition.Condition == "no-issues" && hasIssues {
			_, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET status='Skipped' WHERE id=$1", step.ID)
			return err
		}
		if (definition.HumanGate || p.ManualGates) && !step.Approved {
			_, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET status='WaitingForUser' WHERE id=$1", step.ID)
			if err != nil {
				return err
			}
			_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='WaitingForUser',error='请审核并批准下一步骤' WHERE id=$1", id)
			return err
		}
		intent := "production:" + id + ":" + step.ChapterID + ":" + step.StepID
		var taskID string
		if err = s.DB.QueryRow(ctx, "SELECT id FROM wb_runs WHERE intent_id=$1", intent).Scan(&taskID); err == nil {
			_, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET task_run_id=$2,status='Running' WHERE id=$1", step.ID, taskID)
			return err
		}
		if err != pgx.ErrNoRows {
			return err
		}
		request := RunRequest{IgnoreMemory: ignoreMemory, IntentID: intent, TaskType: definition.Type, PromptTemplateID: definition.PromptTemplateID, AgentID: definition.AgentID, MCPCalls: definition.MCPCalls, Instructions: "工作流步骤：" + p.Workflow.Name + " / " + definition.ID}
		if definition.PromptTemplateID != "" {
			template, e := s.ResolvePrompt(ctx, definition.Type, definition.PromptTemplateID)
			if e != nil {
				return e
			}
			request.Instructions += "\n" + template.Prompt
		}
		if definition.AgentID != "" {
			agent, e := s.GetAgent(ctx, definition.AgentID)
			if e != nil {
				return e
			}
			if agent.TaskType != definition.Type {
				return fmt.Errorf("Agent任务类型与步骤不符")
			}
			request.Instructions += "\n按Agent职责执行：" + agent.Name
		}
		for _, dep := range dependencies {
			request.Instructions += "\n依赖步骤产物（候选资料，未自动定稿）：\n" + dep.Output.Text
			if definition.Type == "revision" && len(dep.Output.Issues) > 0 {
				request.SelectedIssues = append(request.SelectedIssues, dep.Output.Issues...)
			}
			if definition.InputSelection == "previous-candidate" && (definition.Type == "review" || definition.Type == "revision" || definition.Type == "reader" || definition.Type == "developmental") {
				if dep.Type == "draft" || dep.Type == "revision" {
					request.CandidateRunID = dep.ID
				} else if dep.Input.CandidateRunID != "" {
					request.CandidateRunID = dep.Input.CandidateRunID
				}
			}
		}
		run, e := s.SubmitRun(executionCtx, step.ChapterID, request)
		if e != nil {
			_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='WaitingForUser',error=$2 WHERE id=$1", id, e.Error())
			return err
		}
		// SubmitRun checked production budgets in the transaction that made this
		// task visible. Rechecking after a Worker starts could cancel valid work.
		_, err = s.DB.Exec(ctx, "UPDATE wb_production_steps SET task_run_id=$2,status='Running' WHERE id=$1", step.ID, run.ID)
		return err
	}
	if complete {
		_, err = s.DB.Exec(ctx, "UPDATE wb_productions SET status='Succeeded',finished_at=now(),error='' WHERE id=$1", id)
	}
	return err
}
func (s *Service) productionBudget(ctx context.Context, p ProductionRun, step WorkflowStep, run Run) error {
	estimate := requestEstimate(run.Input)
	if step.MaxCost != nil && (estimate == nil || *estimate > *step.MaxCost) {
		return fmt.Errorf("工作流步骤预算不足或价格未知")
	}
	if p.MaxCost == nil {
		return nil
	}
	if estimate == nil || run.Input.Policy.Currency != p.Currency {
		return fmt.Errorf("批量预算要求同币种已配置价格")
	}
	total := *estimate
	for _, other := range p.Steps {
		if other.TaskRunID == "" || other.TaskRunID == run.ID {
			continue
		}
		r, err := s.GetRun(ctx, other.TaskRunID)
		if err != nil {
			return err
		}
		if r.AttemptCount > 0 {
			if r.Cost == nil {
				return fmt.Errorf("批量已有未知费用，需先核对")
			}
			if r.Input.Policy.Currency != p.Currency {
				return fmt.Errorf("批量币种不能混合")
			}
			total += *r.Cost
		} else if r.Status == "Pending" || r.Status == "Running" || r.Status == "RetryScheduled" {
			cost := requestEstimate(r.Input)
			if cost == nil {
				return fmt.Errorf("批量排队费用未知")
			}
			total += *cost
		}
	}
	if total > *p.MaxCost {
		return fmt.Errorf("达到批量总预算，已停止后续请求")
	}
	return nil
}

// CheckAutomationBudgetTx runs inside SubmitRun before its task is visible to
// any Worker. Budget checks after dispatch would race another process.
func (s *Service) CheckAutomationBudgetTx(ctx context.Context, tx pgx.Tx, intent string, input RunInput) error {
	if !strings.HasPrefix(intent, "production:") {
		return nil
	}
	parts := strings.Split(intent, ":")
	if len(parts) != 4 {
		return fmt.Errorf("批量意图格式无效")
	}
	var definitionRaw, chapterRaw []byte
	var maxCost *float64
	var currency, status string
	var manual bool
	if err := tx.QueryRow(ctx, "SELECT workflow_snapshot,chapter_ids,max_cost,currency,status,manual_gates FROM wb_productions WHERE id=$1 AND project_id=$2 FOR UPDATE", parts[1], input.ProjectID).Scan(&definitionRaw, &chapterRaw, &maxCost, &currency, &status, &manual); err != nil {
		return noRows(err)
	}
	if status != "Running" || parts[2] != input.ChapterID {
		return &Conflict{Message: "批量已暂停、取消或章节不符"}
	}
	var definition WorkflowDefinition
	if err := json.Unmarshal(definitionRaw, &definition); err != nil {
		return err
	}
	var selected *WorkflowStep
	for _, step := range definition.Steps {
		if step.ID == parts[3] {
			copy := step
			selected = &copy
		}
	}
	if selected == nil {
		return fmt.Errorf("批量步骤不存在")
	}
	var approved bool
	if err := tx.QueryRow(ctx, "SELECT approved FROM wb_production_steps WHERE production_id=$1 AND chapter_id=$2 AND step_id=$3", parts[1], parts[2], parts[3]).Scan(&approved); err != nil {
		return err
	}
	if (manual || selected.HumanGate) && !approved {
		return &Conflict{Message: "批量步骤尚未获得作者批准"}
	}
	estimate := requestEstimate(input)
	if selected.MaxCost != nil && (estimate == nil || *estimate > *selected.MaxCost) {
		return fmt.Errorf("工作流步骤预算不足或价格未知")
	}
	if maxCost == nil {
		return nil
	}
	if estimate == nil || input.Policy.Currency != currency {
		return fmt.Errorf("批量预算要求同币种已配置价格")
	}
	total := *estimate
	rows, err := tx.Query(ctx, "SELECT input,cost,attempt_count,status FROM wb_runs WHERE intent_id LIKE $1 AND intent_id<>$2", "production:"+parts[1]+":%", intent)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var cost *float64
		var attempts int
		var status string
		if err = rows.Scan(&raw, &cost, &attempts, &status); err != nil {
			return err
		}
		var prior RunInput
		if err = json.Unmarshal(raw, &prior); err != nil {
			return err
		}
		if prior.Policy.Currency != currency {
			return fmt.Errorf("批量不同币种不能混加")
		}
		if attempts > 0 {
			if cost == nil {
				return fmt.Errorf("批量已有未知费用，需先核对")
			}
			total += *cost
		}
		if status == "Pending" || status == "Running" || status == "RetryScheduled" {
			reserve := requestEstimate(prior)
			if reserve == nil {
				return fmt.Errorf("批量排队费用未知")
			}
			total += *reserve
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if total > *maxCost {
		return fmt.Errorf("达到批量总预算，未发送后续请求")
	}
	return nil
}
