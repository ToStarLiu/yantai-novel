package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"novel/internal/job"
	"novel/internal/llm"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func copilotFixture(t *testing.T) (*Service, Chapter, CopilotSession) {
	t.Helper()
	s, c := runtimeFixture(t)
	session, e := s.CreateCopilotSession(context.Background(), c.ProjectID, CopilotSessionInput{Title: "连贯写作讨论", ChapterID: c.ID})
	if e != nil {
		t.Fatal(e)
	}
	return s, c, session
}
func submitCopilot(t *testing.T, s *Service, c Chapter, session CopilotSession) CopilotTurn {
	t.Helper()
	v, e := s.SubmitCopilotTurn(context.Background(), session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "先读取本章，解释人物动机", ChapterID: c.ID, InputRevisionID: c.CurrentRevisionID})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func runningCopilot(t *testing.T, s *Service, turn CopilotTurn) CopilotTurn {
	t.Helper()
	turn.LeaseOwner = job.NewID()
	turn.Status = "Running"
	if _, e := s.DB.Exec(context.Background(), "UPDATE wb_copilot_turns SET status='Running',lease_owner=$2,lease_until=now()+interval '30 seconds' WHERE id=$1", turn.ID, turn.LeaseOwner); e != nil {
		t.Fatal(e)
	}
	return turn
}

func TestCopilotPersistentPrivateSessionsAndConcurrentIntent(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	req := CopilotTurnRequest{IntentID: job.NewID(), Text: "讨论动机", ChapterID: c.ID, InputRevisionID: c.CurrentRevisionID}
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			turn, e := s.SubmitCopilotTurn(ctx, session.ID, req)
			ids <- turn.ID
			errs <- e
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	first := ""
	for id := range ids {
		if first != "" && first != id {
			t.Fatal("duplicate intent created two turns")
		}
		first = id
	}
	req.Text = "更换同一意图"
	if _, e := s.SubmitCopilotTurn(ctx, session.ID, req); !IsConflict(e) {
		t.Fatal("intent conflict missing", e)
	}
	if _, e := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "不能并发"}); !IsConflict(e) {
		t.Fatal("active session conflict missing", e)
	}
	other := WithPrincipal(ctx, Principal{ID: "different", Admin: true})
	if _, e := s.GetCopilotSession(other, session.ID); !errors.Is(e, ErrForbidden) {
		t.Fatal("private conversation exposed", e)
	}
	list, e := s.ListCopilotSessions(other, c.ProjectID)
	if e != nil || len(list) != 0 {
		t.Fatal("private session listed", list, e)
	}
	if _, e = s.CancelCopilotTurn(ctx, first); e != nil {
		t.Fatal(e)
	}
	view, e := s.GetCopilotSession(ctx, session.ID)
	if e != nil || len(view.Turns) != 1 || view.Turns[0].Status != "Cancelled" {
		t.Fatal("cancel not durable", view, e)
	}
}

func TestCopilotAuthorizeSharesBudgetWithExistingRunsAndFreezesPolicy(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	price := 1000000.0
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: s.Model, Currency: "TEST", InputPricePerMillion: &price, OutputPricePerMillion: &price}}
	cap := 1000.0
	if _, e := s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: &cap, Currency: "TEST", MaxOutputTokens: 256}); e != nil {
		t.Fatal(e)
	}
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	s.ModelPolicies["default"] = ModelPolicy{Model: s.Model}
	id, e := s.copilotAuthorize(ctx, turn, copilotWire{InputTokensEstimate: 10, MaxOutputTokens: 256, Messages: json.RawMessage(`[]`), Kind: "generation"})
	if e != nil {
		t.Fatal(e)
	}
	// Existing one-shot paths must reserve against the active Copilot call.
	_, e = s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "review", InputRevisionID: c.CurrentRevisionID})
	if e == nil {
		t.Fatal("unpriced old path bypassed project budget")
	}
	if e = s.copilotUsage(ctx, turn, copilotWire{AttemptID: id, Status: "Succeeded", Usage: llm.Usage{Reported: true, InputTokens: 10, OutputTokens: 20}, Message: json.RawMessage(`{"role":"assistant","content":[]}`)}); e != nil {
		t.Fatal(e)
	}
	cost, e := s.CostSummary(ctx, c.ProjectID)
	if e != nil || cost.KnownEstimatedCost != 30 || cost.InputTokens != 10 || cost.UnknownCalls != 0 {
		t.Fatal("copilot usage not in ledger", cost, e)
	}
	attempts, e := s.CopilotAttempts(ctx, turn.ID)
	if e != nil || len(attempts) != 1 || attempts[0].Policy.Currency != "TEST" {
		t.Fatal("policy snapshot changed", attempts, e)
	}
	next := s.ModelPolicies["default"]
	next.Currency = "OTHER"
	next.InputPricePerMillion = &price
	next.OutputPricePerMillion = &price
	s.ModelPolicies["default"] = next
	if _, e = s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: &cap, Currency: "OTHER", MaxOutputTokens: 256}); e == nil {
		t.Fatal("existing copilot currency was ignored")
	}
}

func TestCopilotAndOneShotRequestsReserveOneSharedProjectBudget(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	price := 1000000.0
	s.ModelPolicies = map[string]ModelPolicy{"default": {Model: s.Model, Currency: "TEST", InputPricePerMillion: &price, OutputPricePerMillion: &price}}
	// Let a prepared existing run reserve its exact known estimate.
	queued, e := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "review", InputRevisionID: c.CurrentRevisionID})
	if e != nil {
		t.Fatal(e)
	}
	base := requestEstimate(queued.Input)
	if base == nil {
		t.Fatal("missing fixture estimate")
	}
	budget := *base + 1
	if _, e = s.SavePreferences(ctx, c.ProjectID, Preferences{MaxCost: &budget, Currency: "TEST", MaxOutputTokens: 256}); e != nil {
		t.Fatal(e)
	}
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	msg := copilotWire{InputTokensEstimate: 10, MaxOutputTokens: 256, Messages: json.RawMessage(`[]`)}
	if _, e = s.copilotAuthorize(ctx, turn, msg); e == nil {
		t.Fatal("Copilot ignored queued one-shot reservation")
	}
	if _, e = s.RunAction(ctx, queued.ID, "cancel"); e != nil {
		t.Fatal(e)
	}
	// Once cancelled, the same fresh request is permitted. The call is still only
	// authorized, so its reservation must in turn block a new old-style request.
	if _, e = s.copilotAuthorize(ctx, turn, msg); e != nil {
		t.Fatal("released reservation not reused", e)
	}
	_, e = s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "review", InputRevisionID: c.CurrentRevisionID})
	if e == nil {
		t.Fatal("old run ignored active Copilot reservation")
	}
}

func TestCopilotSnapshotChangeBlocksFurtherCallsAndToolWrites(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	if _, e := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("作者又改了正文。"), Source: "author"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.copilotAuthorize(ctx, turn, copilotWire{InputTokensEstimate: 10, MaxOutputTokens: 256, Messages: json.RawMessage(`[]`)}); !IsConflict(e) {
		t.Fatal("old snapshot allowed a model call", e)
	}
	var count int
	if e := s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_copilot_attempts WHERE turn_id=$1", turn.ID).Scan(&count); e != nil || count != 0 {
		t.Fatal("attempt dispatched despite invalid snapshot", e, count)
	}
}

func TestCopilotRecoveryNeverRepeatsUnknownModelCalls(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	id, e := s.copilotAuthorize(ctx, turn, copilotWire{InputTokensEstimate: 10, MaxOutputTokens: 256, Messages: json.RawMessage(`[]`)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, "UPDATE wb_copilot_turns SET lease_until=now()-interval '1 second' WHERE id=$1", turn.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.recoverCopilot(ctx); e != nil {
		t.Fatal(e)
	}
	result, e := s.GetCopilotTurn(ctx, turn.ID)
	if e != nil || result.Status != "NeedsReconciliation" {
		t.Fatal("unknown result replayed", result, e)
	}
	var status string
	if e = s.DB.QueryRow(ctx, "SELECT status FROM wb_copilot_attempts WHERE id=$1", id).Scan(&status); e != nil || status != "NeedsReconciliation" {
		t.Fatal(status, e)
	}
	if e = s.recoverCopilot(ctx); e != nil {
		t.Fatal(e)
	}
	events, e := s.CopilotEvents(ctx, turn.ID, 0)
	if e != nil || len(events) < 2 {
		t.Fatal("recovery not inspectable", events, e)
	}
}

func TestCopilotWorkerDoesNotClaimOtherProviderOrChangedModel(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn := submitCopilot(t, s, c, session)
	for _, input := range []CopilotInput{func() CopilotInput { x := turn.Input; x.ProviderID = "configured"; return x }(), func() CopilotInput { x := turn.Input; x.Policy.Model = "different-model"; return x }()} {
		raw, _ := json.Marshal(input)
		if _, e := s.DB.Exec(ctx, "UPDATE wb_copilot_turns SET input=$2 WHERE id=$1", turn.ID, raw); e != nil {
			t.Fatal(e)
		}
		if e := s.AdvanceCopilot(ctx); e != nil {
			t.Fatal(e)
		}
		v, e := s.GetCopilotTurn(ctx, turn.ID)
		if e != nil || v.Status != "Pending" || v.LeaseOwner != "" {
			t.Fatal("worker stole incompatible turn", v, e)
		}
	}
	// A model explicitly configured for this provider is compatible even if the
	// instance's default model differs; the claim retains the saved model.
	s.ModelPolicies = map[string]ModelPolicy{"copilot": {Model: "different-model"}}
	tempEntry := filepath.Join(t.TempDir(), "missing-harness.js")
	t.Setenv("HARNESS_ENTRY_PATH", tempEntry)
	if e := s.AdvanceCopilot(ctx); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, e := s.GetCopilotTurn(ctx, turn.ID)
		if e != nil {
			t.Fatal(e)
		}
		if v.Status == "Failed" {
			if !strings.Contains(v.Error, "Pi运行环境") {
				t.Fatal(v)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("compatible routed model was not claimed")
}

func TestCopilotSubprocessProtocolPersistsToolsMessagesUsageAndHistoryKey(t *testing.T) {
	node, e := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(node); e != nil {
		t.Skip("Node fixture runtime unavailable")
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	script := `const rl=require('node:readline').createInterface({input:process.stdin});let phase=0,attempt;function emit(v){console.log(JSON.stringify(v))};rl.on('line',line=>{const m=JSON.parse(line);if(phase===0){if(!m.model.fake)process.exit(10);phase=1;emit({type:'authorize',requestId:'a',inputTokensEstimate:10,maxOutputTokens:256,messages:[]});}else if(phase===1){if(m.error)process.exit(11);attempt=m.result.attemptId;phase=2;emit({type:'usage',requestId:'u',attemptId:attempt,status:'Succeeded',usage:{inputTokens:10,outputTokens:7,reported:true},message:{role:'assistant',content:[{type:'text',text:'先读取正文'}]}});}else if(phase===2){phase=3;emit({type:'tool',requestId:'t',name:'read_chapter',args:{},toolCallId:'call1'});}else if(phase===3){if(m.error)process.exit(12);emit({type:'delta',text:'已核对左臂伤势。'});emit({type:'done',messages:[{role:'user',content:'讨论动机'},{role:'assistant',content:[{type:'text',text:'已核对左臂伤势。'}]}],contextStats:{usedTokens:17}});}});`
	file := filepath.Join(t.TempDir(), "harness.cjs")
	if e = os.WriteFile(file, []byte(script), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HARNESS_ENTRY_PATH", file)
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	if e = s.ExecuteCopilotTurn(ctx, turn); e != nil {
		t.Fatal(e)
	}
	result, e := s.GetCopilotTurn(ctx, turn.ID)
	if e != nil || result.Status != "Succeeded" || result.Text != "已核对左臂伤势。" {
		t.Fatalf("protocol failed %#v %v", result, e)
	}
	view, e := s.GetCopilotSession(ctx, session.ID)
	if e != nil || !strings.Contains(string(view.Messages), "已核对左臂伤势") || view.Session.ContextKey != turn.Input.ContextKey || view.Session.Version != 1 {
		t.Fatal("history/context key not durable", view, e)
	}
	events, e := s.CopilotEvents(ctx, turn.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	tools := 0
	for _, event := range events {
		if event.Kind == "tool-result" {
			tools++
		}
	}
	if tools != 1 {
		t.Fatal("tool result missing", events)
	}
	if events[len(events)-1].Kind != "completed" {
		t.Fatal("terminal event missing")
	}
	after, e := s.CopilotEvents(ctx, turn.ID, events[len(events)-2].ID)
	if e != nil || len(after) != 1 {
		t.Fatal("event replay cursor failed", after, e)
	}
}

func TestCopilotCancellationKeepsChargedAttemptUnknown(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	if _, e := s.copilotAuthorize(ctx, turn, copilotWire{InputTokensEstimate: 10, MaxOutputTokens: 256, Messages: json.RawMessage(`[]`)}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.CancelCopilotTurn(ctx, turn.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.finishCopilot(ctx, turn, "Cancelled", "用户停止", nil); e != nil {
		t.Fatal(e)
	}
	attempts, e := s.CopilotAttempts(ctx, turn.ID)
	if e != nil || len(attempts) != 1 || attempts[0].Status != "NeedsReconciliation" {
		t.Fatal("cancelled charged call lost", attempts, e)
	}
	cost, e := s.CostSummary(ctx, c.ProjectID)
	if e != nil || cost.UnknownCalls != 1 {
		t.Fatal("unknown treated free", cost, e)
	}
}

func TestCopilotRealPiHarnessToolsCandidateAndContinuedConversation(t *testing.T) {
	node, e := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	if e != nil {
		t.Fatal(e)
	}
	entry, e := filepath.Abs("../../harness/dist/main.js")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(entry); e != nil {
		t.Skip("Build Pi harness before integration test")
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn, e := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "请润色这段正文，生成一个可比较的改写候选。", ChapterID: c.ID, InputRevisionID: c.CurrentRevisionID, Selection: "林川左臂受伤。"})
	if e != nil {
		t.Fatal(e)
	}
	turn = runningCopilot(t, s, turn)
	if e = s.ExecuteCopilotTurn(ctx, turn); e != nil {
		t.Fatal(e)
	}
	v, e := s.GetCopilotTurn(ctx, turn.ID)
	if e != nil || v.Status != "Succeeded" {
		t.Fatalf("real Pi turn failed: %#v %v", v, e)
	}
	var artifacts []CopilotArtifact
	if e = json.Unmarshal(v.Artifacts, &artifacts); e != nil {
		t.Fatal(e)
	}
	if len(artifacts) == 0 {
		t.Fatalf("real Pi produced no candidate artifacts: %#v", v)
	}
	view, e := s.GetCopilotSession(ctx, session.ID)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(view.Messages), "toolResult") {
		t.Fatalf("Pi tool execution not persisted: %s", view.Messages)
	}
	attempts, e := s.CopilotAttempts(ctx, turn.ID)
	if e != nil || len(attempts) < 2 {
		t.Fatal("Pi did not perform a tool-result continuation", attempts, e)
	}
	current, e := s.GetChapter(ctx, c.ID)
	if e != nil || current.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("candidate overwrote source", current, e)
	}
	follow, e := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "刚才的建议再解释一下。", ChapterID: c.ID, InputRevisionID: c.CurrentRevisionID})
	if e != nil {
		t.Fatal(e)
	}
	follow = runningCopilot(t, s, follow)
	if e = s.ExecuteCopilotTurn(ctx, follow); e != nil {
		t.Fatal(e)
	}
	after, e := s.GetCopilotSession(ctx, session.ID)
	if e != nil || after.Session.Version != 2 {
		t.Fatal("conversation did not resume", after, e)
	}
	if after.Turns[1].Status != "Succeeded" {
		t.Fatal("followup failed", after.Turns[1])
	}
}

func TestCopilotSessionStaleSnapshotAndCompletionLimits(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	if e := s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`{"not":"messages"}`)); e != nil {
		t.Fatal(e)
	}
	v, e := s.GetCopilotSession(ctx, session.ID)
	if e != nil || v.Session.Version != 0 || v.Turns[0].Status != "Failed" {
		t.Fatal("invalid transcript changed canonical session", v, e)
	}
	turn = runningCopilot(t, s, submitCopilot(t, s, c, session))
	if _, e = s.DB.Exec(ctx, "UPDATE wb_copilot_turns SET lease_until=now()-interval '1 second' WHERE id=$1", turn.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`[]`)); e == nil {
		t.Fatal("expired worker committed")
	}
}
