package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"novel/internal/job"
	"novel/internal/llm"
	"novel/internal/review"
)

func requestProse(request llm.Request) string { raw, _ := json.Marshal(request); return string(raw) }
func taskMemoryInput(title, content string) AuthorMemoryInput {
	return AuthorMemoryInput{Scope: "author", Kind: "voice", Title: title, Content: content}
}
func TestAuthorTaskMemoryPrivateProviderContextNeverEntersSharedRequests(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("task-writer")
	secret := "PRIVATE-PERSONAL-TASK: 叙述克制，用动作承载犹豫。"
	note := saveMemory(t, s, ctx, taskMemoryInput("PRIVATE-MEMORY-TITLE", secret))
	bookInput := taskMemoryInput("本书的声音", "PRIVATE-BOOK-TASK: 本书对白保持短促。")
	bookInput.Scope, bookInput.ProjectID = "book", c.ProjectID
	saveMemory(t, s, ctx, bookInput)
	saveMemory(t, s, memoryAuthor("another-writer"), taskMemoryInput("外人偏好", "PRIVATE-FOREIGN-TASK"))
	pending := taskMemoryInput("待确认", "PRIVATE-PENDING-TASK")
	pending.Status = "pending"
	saveMemory(t, s, ctx, pending)
	var sent llm.Request
	s.Provider = &llm.Fake{Handler: func(_ context.Context, request llm.Request) (llm.Response, error) {
		sent = request
		return llm.Response{Text: "林川用右手扶住桥栏，左臂仍然受伤。", Raw: []byte("provider-response")}, nil
	}}
	r, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft", Instructions: "这次允许稍长的句子，保持作者决定。", UseCache: true})
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(r)
	if strings.Contains(string(public), secret) || strings.Contains(string(public), note.Title) || r.Input.OwnerID != "task-writer" || r.Input.UseCache || r.Input.AuthorContextBytes <= 0 {
		t.Fatal("private context leaked or cache not disabled", string(public))
	}
	if r.Input.AuthorMemory == nil || r.Input.AuthorMemory.OwnerID != "task-writer" {
		t.Fatal("missing owner baseline")
	}
	private, err := s.GetAuthorTaskContext(ctx, r.ID)
	if err != nil || len(private.Memory.Items) != 2 || !private.Current {
		t.Fatal("private snapshot unavailable", private, err)
	}
	if _, err = s.GetAuthorTaskContext(memoryAuthor("another-writer"), r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign admin read private snapshot", err)
	}
	if _, err = s.GetAuthorTaskContext(context.Background(), r.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("worker read owner HTTP contract", err)
	}
	claimed := claim(t, s)
	if err = s.ExecuteRun(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	sentText := requestProse(sent)
	if !strings.Contains(sentText, secret) || !strings.Contains(sentText, bookInput.Content) || strings.Contains(sentText, "PRIVATE-FOREIGN-TASK") || strings.Contains(sentText, "PRIVATE-PENDING-TASK") {
		t.Fatal("provider did not receive exact selected owner context", sentText)
	}
	if !strings.Contains(sent.Messages[0].Content, "本次明确要求优先") || !strings.Contains(sentText, "这次允许稍长的句子") {
		t.Fatal("authority boundaries missing")
	}
	if strings.Contains(requestProse(r.Input.Request), "私人作者记忆") {
		t.Fatal("augmented system mutated shared slice")
	}
	var attempt []byte
	if err = s.DB.QueryRow(ctx, "SELECT request_snapshot FROM wb_run_attempts WHERE run_id=$1", r.ID).Scan(&attempt); err != nil || strings.Contains(string(attempt), secret) || strings.Contains(string(attempt), bookInput.Content) {
		t.Fatal("private prompt entered shared attempt", string(attempt), err)
	}
	if err = s.PutRunCache(ctx, r.Input, RunOutput{Text: "personalized cache attempt"}); err != nil {
		t.Fatal(err)
	}
	if _, hit, e := s.GetRunCache(ctx, r.Input); e != nil || hit {
		t.Fatal("private cache read available", hit, e)
	}
	var cached int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_run_cache").Scan(&cached); err != nil || cached != 0 {
		t.Fatal("personalized output in public cache", cached, err)
	}
	metrics, err := s.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(metrics)
	if strings.Contains(string(raw), secret) {
		t.Fatal("metrics leaked private request")
	}
	exported, err := s.ExportProject(ctx, c.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(exported)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), note.Title) {
		t.Fatal("book package leaked author profile")
	}
	if err = s.DeleteProject(ctx, c.ProjectID); err != nil {
		t.Fatal(err)
	}
	var contexts int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_run_author_contexts WHERE run_id=$1", r.ID).Scan(&contexts); err != nil || contexts != 0 {
		t.Fatal("orphan private task context after book deletion", contexts, err)
	}
}

func TestAuthorTaskMemoryEligibilityAndSingleTaskOptOut(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("eligible-writer")
	secret := "PRIVATE-ELIGIBLE: 多用动作，避免段尾结论。"
	saveMemory(t, s, ctx, taskMemoryInput("声音", secret))
	for _, kind := range []string{"draft", "planning", "rewrite", "revision", "translation", "reader", "developmental", "publisher", "review", "analysis", "voice", "handoff"} {
		request := RunRequest{IntentID: job.NewID(), TaskType: kind}
		if kind == "rewrite" {
			request.Selection = "林川左臂受伤。"
			request.Operation = "polish"
		}
		if kind == "revision" {
			request.SelectedIssues = []review.Issue{{Kind: "Continuity", Severity: "Minor", Quote: "林川左臂受伤。", Explanation: "保持伤势", SuggestedAction: "保留"}}
		}
		r, err := s.SubmitRun(ctx, c.ID, request)
		if err != nil {
			t.Fatal(kind, err)
		}
		task, err := s.loadAuthorTask(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		has := strings.Contains(requestProse(task.Request), secret)
		if has != authorTaskEligible(kind) {
			t.Fatal(kind, "unexpected memory use", has)
		}
		if !authorTaskEligible(kind) && (task.ExcludedReason == "" || r.Input.AuthorMemory != nil) {
			t.Fatal("evidence task got personal baseline", kind)
		}
	}
	r, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft", IgnoreMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.GetAuthorTaskContext(ctx, r.ID)
	if err != nil || !task.IgnoreMemory || task.ExcludedReason == "" || len(task.Memory.Items) != 0 || r.Input.AuthorMemory != nil || r.Input.AuthorContextBytes != 0 {
		t.Fatal("opt-out failed", task, err)
	}
	// Internal task contexts never silently borrow the environment admin profile.
	saveMemory(t, s, memoryAuthor("admin"), taskMemoryInput("环境管理员偏好", "PRIVATE-ADMIN-WORKER"))
	internal, err := s.SubmitRun(context.Background(), c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if internal.Input.OwnerID != "" || internal.Input.AuthorMemory != nil || strings.Contains(requestProse(internal.Input.Request), "PRIVATE-ADMIN-WORKER") {
		t.Fatal("worker inherited admin preferences")
	}
}

func TestAuthorTaskMemoryExcludeFactExtractionAndUnauthorizedAgent(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("evidence-writer")
	saveMemory(t, s, ctx, taskMemoryInput("创作偏好", "PRIVATE-DO-NOT-SHAPE-FACTS"))
	if _, err := s.Finalize(ctx, c.ID, c.CurrentRevisionID); err != nil {
		t.Fatal(err)
	}
	r, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "memory-extraction"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.loadAuthorTask(ctx, r)
	if err != nil || strings.Contains(requestProse(task.Request), "PRIVATE-DO-NOT-SHAPE-FACTS") || r.Input.AuthorMemory != nil {
		t.Fatal("fact extraction received personal taste", err)
	}
	agent, err := s.SaveAgent(ctx, AgentDefinition{Name: "没有记忆权限", TaskType: "planning", MaxOutputTokens: 500, Tools: []string{"create-candidate"}})
	if err != nil {
		t.Fatal(err)
	}
	drafted, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "planning", AgentID: agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	task, err = s.loadAuthorTask(ctx, drafted)
	if err != nil || task.ExcludedReason != "所选Agent未获记忆读取权限" || strings.Contains(requestProse(task.Request), "PRIVATE-DO-NOT-SHAPE-FACTS") {
		t.Fatal("agent permissions bypassed", task, err)
	}
}

func TestAuthorTaskMemoryFrozenVersionBlocksDispatchAndAdoption(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("version-writer")
	note := saveMemory(t, s, ctx, taskMemoryInput("偏好", "PRIVATE-ORIGINAL: 克制。"))
	called := 0
	s.Provider = &llm.Fake{Handler: func(_ context.Context, request llm.Request) (llm.Response, error) {
		called++
		return llm.Response{Text: "新候选", Raw: []byte("ok")}, nil
	}}
	r, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	claimed := claim(t, s)
	content := "PRIVATE-NEW: 对白不解释。"
	note, err = s.UpdateAuthorMemory(ctx, note.ID, AuthorMemoryUpdate{ExpectedVersion: note.Version, Content: &content})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ExecuteRun(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetRun(ctx, r.ID)
	if err != nil || done.Status != "Failed" || !done.Stale || called != 0 {
		t.Fatal("stale memory dispatched", done, called, err)
	}
	if _, err = s.RunAction(ctx, r.ID, "retry"); !IsConflict(err) {
		t.Fatal("old preferences retried", err)
	}
	fresh, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ExecuteRun(context.Background(), claim(t, s)); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteAuthorMemory(ctx, note.ID, note.Version); err != nil {
		t.Fatal(err)
	}
	fresh, err = s.GetRun(ctx, fresh.ID)
	if err != nil || !fresh.Stale {
		t.Fatal("forgotten memory candidate still current", err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("候选"), Source: "ai", RunID: fresh.ID}); !IsConflict(err) {
		t.Fatal("forgotten memory candidate adopted", err)
	}
	private, err := s.GetAuthorTaskContext(ctx, fresh.ID)
	if err != nil || private.Current {
		t.Fatal("private validity ignored forget", private, err)
	}
}

func TestAuthorTaskMemoryAccountAndMemberRevocationPreventsCall(t *testing.T) {
	for _, disable := range []bool{false, true} {
		t.Run(fmt.Sprint("disabled-", disable), func(t *testing.T) {
			s, c := runtimeFixture(t)
			u, ctx := collabUser(t, s, "queued-author")
			if _, err := s.AddMember(context.Background(), c.ProjectID, u.Username, "editor"); err != nil {
				t.Fatal(err)
			}
			saveMemory(t, s, ctx, taskMemoryInput("偏好", "PRIVATE-OWNED"))
			r, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
			if err != nil {
				t.Fatal(err)
			}
			claimed := claim(t, s)
			if disable {
				_, err = s.DB.Exec(ctx, "UPDATE wb_users SET disabled=true WHERE id=$1", u.ID)
			} else {
				_, err = s.DB.Exec(ctx, "DELETE FROM wb_project_members WHERE project_id=$1 AND user_id=$2", c.ProjectID, u.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
				t.Fatal("revoked owner reached model")
				return llm.Response{}, nil
			}}
			if err = s.ExecuteRun(context.Background(), claimed); err != nil {
				t.Fatal(err)
			}
			done, err := s.GetRun(context.Background(), r.ID)
			if err != nil || done.Status != "Failed" {
				t.Fatal(done, err)
			}
		})
	}
}

func TestAuthorTaskMemoryFullPrivateBytesReservedBeforeQueueAndDispatch(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("budget-writer")
	saveMemory(t, s, ctx, taskMemoryInput("长偏好", strings.Repeat("克制，不替读者解释。", 100)))
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: s.Model, InputPricePerMillion: ptr(1), OutputPricePerMillion: ptr(1), Currency: "USD"}}
	probe, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.loadAuthorTask(ctx, probe)
	if err != nil {
		t.Fatal(err)
	}
	full := requestEstimate(probe.Input)
	publicInput := probe.Input
	publicInput.AuthorContextBytes = 0
	publicCost := requestEstimate(publicInput)
	exactCost := estimateCost(probe.Input.Policy, authorRequestBytes(private.Request), int64(private.Request.MaxOutputTokens))
	if full == nil || publicCost == nil || exactCost == nil || *full <= *publicCost || *full != *exactCost {
		t.Fatal("private input bytes underreserved", full, publicCost, exactCost)
	}
	if _, err = s.RunAction(ctx, probe.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	threshold := (*full + *publicCost) / 2
	s.MaxJobCost = &threshold
	var before int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_runs").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"}); err == nil {
		t.Fatal("private memory omitted from submission budget")
	}
	var after int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_runs").Scan(&after); err != nil || after != before {
		t.Fatal("budget rejection left partial task", after, err)
	}
	s.MaxJobCost = nil
	queued, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	claimed := claim(t, s)
	s.MaxJobCost = &threshold
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		t.Fatal("dispatch omitted private reserve")
		return llm.Response{}, nil
	}}
	if err = s.ExecuteRun(context.Background(), claimed); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetRun(ctx, queued.ID)
	if err != nil || done.Status != "Failed" {
		t.Fatal(done, err)
	}
}

func TestAuthorTaskMemoryProductionUsesFrozenOwnerAndStopsWhenPreferencesChange(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("batch-author")
	note := saveMemory(t, s, ctx, taskMemoryInput("批量文风", "PRIVATE-BATCH-OWNER"))
	saveMemory(t, s, memoryAuthor("admin"), taskMemoryInput("管理员", "PRIVATE-BATCH-WRONG-ADMIN"))
	workflow, err := s.SaveWorkflow(context.Background(), WorkflowDefinition{Name: "带作者上下文的批量", Version: 1, Steps: []WorkflowStep{{ID: "write", Type: "draft"}, {ID: "feedback", Type: "reader", DependsOn: []string{"write"}, InputSelection: "previous-candidate"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: workflow.ID, ChapterIDs: []string{c.ID}, MaxChapters: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p.OwnerID != "batch-author" || p.IgnoreMemory {
		t.Fatal("batch creator missing", p)
	}
	// A worker's caller principal must not replace the author captured at creation.
	if err = s.AdvanceAutomations(memoryAuthor("admin")); err != nil {
		t.Fatal(err)
	}
	p, err = s.GetProduction(ctx, p.ID)
	if err != nil || p.Steps[0].TaskRunID == "" {
		t.Fatal(p, err)
	}
	run, err := s.GetRun(ctx, p.Steps[0].TaskRunID)
	if err != nil || run.Input.OwnerID != "batch-author" {
		t.Fatal("worker became owner", run, err)
	}
	s.Provider = &llm.Fake{Handler: func(_ context.Context, request llm.Request) (llm.Response, error) {
		text := requestProse(request)
		if !strings.Contains(text, "PRIVATE-BATCH-OWNER") || strings.Contains(text, "PRIVATE-BATCH-WRONG-ADMIN") {
			t.Fatal("batch used wrong profile", text)
		}
		return llm.Response{Text: "林川仍旧左臂受伤。", Raw: []byte("ok")}, nil
	}}
	if err = s.ExecuteRun(context.Background(), claim(t, s)); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(context.Background()); err != nil {
		t.Fatal(err)
	}
	changed := "PRIVATE-BATCH-CHANGED"
	if _, err = s.UpdateAuthorMemory(ctx, note.ID, AuthorMemoryUpdate{ExpectedVersion: note.Version, Content: &changed}); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, err = s.GetProduction(ctx, p.ID)
	if err != nil || p.Status != "WaitingForUser" || p.Steps[1].TaskRunID != "" {
		t.Fatal("batch silently moved to new profile", p, err)
	}
	if _, err = s.ProductionAction(ctx, p.ID, "resume", ""); !IsConflict(err) {
		t.Fatal("old preference batch resumed", err)
	}
}

func TestAuthorTaskMemoryProductionOptOutAndCandidateOwners(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("multi-author")
	saveMemory(t, s, ctx, taskMemoryInput("声音", "PRIVATE-MULTI-AUTHOR"))
	workflow, err := s.SaveWorkflow(context.Background(), WorkflowDefinition{Name: "批量关闭记忆", Version: 1, Steps: []WorkflowStep{{ID: "write", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: workflow.ID, ChapterIDs: []string{c.ID}, MaxChapters: 1, IgnoreMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, err = s.GetProduction(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.GetAuthorTaskContext(ctx, p.Steps[0].TaskRunID)
	if err != nil || !task.IgnoreMemory || len(task.Memory.Items) != 0 {
		t.Fatal("batch optout not copied", task, err)
	}
	if _, err = s.ProductionAction(memoryAuthor("different-admin"), p.ID, "pause", ""); !errors.Is(err, ErrForbidden) {
		t.Fatal("other admin changed owner-bound batch", err)
	}
	if _, err = s.ProductionAction(ctx, p.ID, "cancel", ""); err != nil {
		t.Fatal(err)
	}
	g, err := s.StartCandidates(ctx, c.ID, CandidateRequest{Count: 2, InputRevisionID: c.CurrentRevisionID})
	if err != nil {
		t.Fatal(err)
	}
	if g.OwnerID != "multi-author" {
		t.Fatal("candidate creator missing")
	}
	s.Provider = &llm.Fake{Handler: func(_ context.Context, request llm.Request) (llm.Response, error) {
		if len(request.ResponseSchema) > 0 {
			if strings.Contains(requestProse(request), "PRIVATE-MULTI-AUTHOR") {
				t.Fatal("ranking evidence polluted by taste")
			}
			return llm.Response{Text: `{"issues":[]}`, Raw: []byte("ok")}, nil
		}
		if !strings.Contains(requestProse(request), "PRIVATE-MULTI-AUTHOR") {
			t.Fatal("candidate missed owner profile")
		}
		return llm.Response{Text: "林川左臂仍然受伤。", Raw: []byte("ok")}, nil
	}}
	for i := 0; i < 12; i++ {
		if err = s.AdvanceCandidates(memoryAuthor("different-admin")); err != nil {
			t.Fatal(err)
		}
		claimed, ok, e := s.ClaimRun(context.Background(), "multi-test-worker")
		if e != nil {
			t.Fatal(e)
		}
		if ok {
			if e = s.ExecuteRun(context.Background(), claimed); e != nil {
				t.Fatal(e)
			}
		}
	}
	g, err = s.GetCandidateGroup(ctx, g.ID)
	if err != nil || g.Status != "Succeeded" || len(g.Rankings) != 2 {
		t.Fatal(g, err)
	}
	for _, rank := range g.Rankings {
		for _, id := range []string{rank.RunID, rank.ReviewRunID} {
			r, e := s.GetRun(ctx, id)
			if e != nil || r.Input.OwnerID != "multi-author" {
				t.Fatal("candidate branch lost creating owner", r, e)
			}
		}
	}
}

func TestAuthorTaskMemoryBatchCannotBorrowWorkerAdminWithoutCreator(t *testing.T) {
	s, c := runtimeFixture(t)
	saveMemory(t, s, memoryAuthor("admin"), taskMemoryInput("环境偏好", "PRIVATE-NEVER-BORROW"))
	workflow, err := s.SaveWorkflow(context.Background(), WorkflowDefinition{Name: "旧批量", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartProduction(context.Background(), c.ProjectID, ProductionRequest{WorkflowID: workflow.ID, ChapterIDs: []string{c.ID}, MaxChapters: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(memoryAuthor("admin")); err != nil {
		t.Fatal(err)
	}
	p, err = s.GetProduction(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.GetRun(context.Background(), p.Steps[0].TaskRunID)
	if err != nil || r.Input.OwnerID != "" || r.Input.AuthorMemory != nil {
		t.Fatal("legacy batch borrowed worker profile", r, err)
	}
}

func TestAuthorTaskMemoryReadOnlyOwnerCanInspectButCannotDispatchAndPrivateBytesAreFenced(t *testing.T) {
	s, c := runtimeFixture(t)
	u, ctx := collabUser(t, s, "memory-history-author")
	if _, err := s.AddMember(context.Background(), c.ProjectID, u.Username, "editor"); err != nil {
		t.Fatal(err)
	}
	saveMemory(t, s, ctx, taskMemoryInput("偏好", "PRIVATE-READONLY-HISTORY"))
	r, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE wb_project_members SET role='viewer' WHERE project_id=$1 AND user_id=$2", c.ProjectID, u.ID); err != nil {
		t.Fatal(err)
	}
	historical, err := s.GetAuthorTaskContext(ctx, r.ID)
	if err != nil || len(historical.Memory.Items) != 1 {
		t.Fatal("owner lost access to private history after readonly change", historical, err)
	}
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		t.Fatal("readonly creator dispatched task")
		return llm.Response{}, nil
	}}
	if err = s.ExecuteRun(context.Background(), claim(t, s)); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetRun(context.Background(), r.ID)
	if err != nil || done.Status != "Failed" {
		t.Fatal("readonly task not blocked", done, err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE wb_project_members SET role='editor' WHERE project_id=$1 AND user_id=$2", c.ProjectID, u.ID); err != nil {
		t.Fatal(err)
	}
	other, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE wb_run_author_contexts SET augmented_request=jsonb_set(augmented_request,'{Messages}',(augmented_request->'Messages')||'[ {"role":"user","content":"unbudgeted-private-material"} ]'::jsonb) WHERE run_id=$1`, other.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ExecuteRun(context.Background(), claim(t, s)); err != nil {
		t.Fatal(err)
	}
	other, err = s.GetRun(context.Background(), other.ID)
	if err != nil || other.Status != "Failed" {
		t.Fatal("corrupt private request bypassed reserve fence", other, err)
	}
}

func TestAuthorTaskMemoryGlobalSwitchAndCompletionRecheck(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := memoryAuthor("switch-author")
	note := saveMemory(t, s, ctx, taskMemoryInput("文风", "PRIVATE-SWITCH-ON"))
	prefs, err := s.GetAuthorMemorySettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateAuthorMemorySettings(ctx, AuthorMemorySettingsUpdate{ExpectedVersion: prefs.Version, UseMemory: false, SuggestMemories: true}); err != nil {
		t.Fatal(err)
	}
	paused, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.loadAuthorTask(ctx, paused)
	if err != nil || private.ExcludedReason != "作者已暂停使用记忆" || strings.Contains(requestProse(private.Request), note.Content) {
		t.Fatal("global pause ignored", private, err)
	}
	if _, err = s.RunAction(ctx, paused.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	prefs, err = s.GetAuthorMemorySettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateAuthorMemorySettings(ctx, AuthorMemorySettingsUpdate{ExpectedVersion: prefs.Version, UseMemory: true, SuggestMemories: true}); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	s.Provider = &llm.Fake{Handler: func(_ context.Context, request llm.Request) (llm.Response, error) {
		if !strings.Contains(requestProse(request), note.Content) {
			t.Fatal("fresh task ignored active profile")
		}
		if e := s.DeleteAuthorMemory(ctx, note.ID, note.Version); e != nil {
			t.Fatal(e)
		}
		return llm.Response{Text: "任务期间偏好改变后的候选", Raw: []byte("received")}, nil
	}}
	if err = s.ExecuteRun(context.Background(), claim(t, s)); err != nil {
		t.Fatal(err)
	}
	fresh, err = s.GetRun(ctx, fresh.ID)
	if err != nil || fresh.Status != "Succeeded" || !fresh.Stale {
		t.Fatal("midflight forget not marked stale", fresh, err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Source: "ai", RunID: fresh.ID, Document: DocumentFromText(fresh.Output.Text)}); !IsConflict(err) {
		t.Fatal("midflight stale candidate adopted", err)
	}
}
