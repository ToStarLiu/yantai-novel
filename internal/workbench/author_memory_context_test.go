package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"novel/internal/job"
)

func memoryTurnScope(t *testing.T, s *Service, turn CopilotTurn) CopilotToolContext {
	t.Helper()
	scope, err := s.NewCopilotToolContext(copilotAdmin(), turn.ID, turn.ProjectID, turn.Input.ChapterID, turn.Input.InputRevisionID, turn.Input.Selection)
	if err != nil {
		t.Fatal(err)
	}
	scope.AuthorMemory, scope.IgnoreMemory, scope.SuggestMemories = turn.Input.AuthorMemory, turn.Input.IgnoreMemory, turn.Input.SuggestMemories
	return scope
}

func TestAuthorMemoryCopilotPrivatePagingAndCandidateAdoptionFence(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	content := "PRIVATE-AUTHOR-PROSE:" + strings.Repeat("通过动作呈现情绪，不用抽象概括。", 55) + "最后保留留白。"
	v := saveMemory(t, s, ctx, memoryInput("动作与留白", content))
	other := saveMemory(t, s, memoryAuthor("another-owner"), memoryInput("别人的风格", "PRIVATE-OTHER: 禁止注入"))
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	if turn.Input.AuthorMemory == nil || len(turn.Input.AuthorMemory.Items) != 1 || turn.Input.AuthorMemory.Items[0].ID != v.ID {
		t.Fatal("private context missing or crossed authors", turn.Input.AuthorMemory)
	}
	scope := memoryTurnScope(t, s, turn)
	var full strings.Builder
	for offset := 0; offset < len([]rune(content)); {
		result, err := s.ExecuteCopilotTool(ctx, scope, "read_author_memory", copilotArgs(map[string]any{"memoryId": v.ID, "offset": offset, "maxChars": 200}))
		if err != nil {
			t.Fatal(err)
		}
		page := result.Content.(map[string]any)
		full.WriteString(page["text"].(string))
		if page["nextOffset"].(int) != offset+len([]rune(page["text"].(string))) {
			t.Fatal("memory pagination lost its source cursor")
		}
		offset = page["nextOffset"].(int)
	}
	if full.String() != content {
		t.Fatal("memory text was lost in paging")
	}
	if _, err := s.ExecuteCopilotTool(ctx, scope, "read_author_memory", copilotArgs(map[string]any{"memoryId": other.ID})); !errors.Is(err, ErrNotFound) {
		t.Fatal("another author's memory escaped the frozen directory", err)
	}
	result, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"text":"她把杯子推开，没有回答。","mode":"chapter"}`))
	if err != nil {
		t.Fatal(err)
	}
	runID := result.Artifact.RunID
	var raw []byte
	if err = s.DB.QueryRow(ctx, "SELECT input FROM wb_runs WHERE id=$1", runID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE-") || strings.Contains(string(raw), "动作与留白") || strings.Contains(string(raw), "sourceQuote") {
		t.Fatal("shared run retained private author memory prose", string(raw))
	}
	run, err := s.GetRun(ctx, runID)
	if err != nil || run.Input.AuthorMemory == nil || run.Input.AuthorMemory.OwnerID != "admin" {
		t.Fatal("shared candidate lost validity fence", err)
	}
	next := "让动作承担情绪，但本次可以直说关键事实。"
	if _, err = s.UpdateAuthorMemory(ctx, v.ID, AuthorMemoryUpdate{ExpectedVersion: v.Version, Content: &next}); err != nil {
		t.Fatal(err)
	}
	readRun, err := s.GetRun(ctx, runID)
	if err != nil || !readRun.Stale {
		t.Fatal("candidate getter hid a changed memory baseline", readRun.Stale, err)
	}
	listed, err := s.ListRuns(ctx, c.ProjectID)
	if err != nil || len(listed) != 1 || !listed[0].Stale {
		t.Fatal("candidate list hid a changed memory baseline", listed, err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_author_memory", []byte(`{}`)); !IsConflict(err) {
		t.Fatal("old private memory survived an edit", err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(result.Artifact.Text), Source: "ai", RunID: runID}); !IsConflict(err) {
		t.Fatal("candidate silently applied an outdated preference", err)
	}
}

func TestAuthorMemoryCopilotIgnoreBypassesPreferencesAndSuggestions(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	v := saveMemory(t, s, ctx, memoryInput("通用文风", "PRIVATE-IGNORE: 不要解释潜台词。"))
	request := CopilotTurnRequest{IntentID: job.NewID(), Text: "我喜欢简短的对白，以后请先展示差异。", ChapterID: c.ID, IgnoreMemory: true}
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	turn = runningCopilot(t, s, turn)
	if turn.Input.AuthorMemory == nil || turn.Input.AuthorMemory.Enabled || len(turn.Input.AuthorMemory.Items) != 0 || turn.Input.SuggestMemories {
		t.Fatal("ignore switch froze active private preferences", turn.Input.AuthorMemory)
	}
	scope := memoryTurnScope(t, s, turn)
	if err = s.DeleteAuthorMemory(ctx, v.ID, v.Version); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"read_author_memory", "propose_author_memory"} {
		if _, err = s.ExecuteCopilotTool(ctx, scope, tool, []byte(`{}`)); !errors.Is(err, ErrForbidden) {
			t.Fatal("ignored memory tool remained available", tool, err)
		}
	}
	result, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"text":"本轮按当前要求直接说明。","mode":"chapter"}`))
	if err != nil {
		t.Fatal("private preference edit invalidated an ignored round", err)
	}
	run, err := s.GetRun(ctx, result.Artifact.RunID)
	if err != nil || run.Input.AuthorMemory != nil {
		t.Fatal("ignored candidate contains memory dependency", err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(result.Artifact.Text), Source: "ai", RunID: result.Artifact.RunID}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListAuthorMemories(ctx, "", "")
	if err != nil || len(list.Items) != 0 {
		t.Fatal("ignored turn proposed a new memory", list, err)
	}
}

func TestAuthorMemoryCopilotProposalsStayPendingRespectStopAndCurrentSwitch(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	text := "我喜欢克制的描写，人物对话不要解释已经知道的事。"
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: text})
	if err != nil {
		t.Fatal(err)
	}
	turn = runningCopilot(t, s, turn)
	scope := memoryTurnScope(t, s, turn)
	args := copilotArgs(map[string]any{"scope": "author", "kind": "voice", "title": "克制表达", "content": "克制描写，避免解释已知事实。", "sourceQuote": "我喜欢克制的描写"})
	result, err := s.ExecuteCopilotTool(ctx, scope, "propose_author_memory", args)
	if err != nil || result.Artifact == nil || result.Artifact.Kind != "author-memory-proposal" || result.Artifact.Status != "pending" {
		t.Fatal("agent activated memory instead of proposing it", result, err)
	}
	snap, err := s.ResolveAuthorMemories(ctx, c.ProjectID, text)
	if err != nil || len(snap.Items) != 0 {
		t.Fatal("pending memory entered effective context", snap, err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_chapter", []byte(`{}`)); err != nil {
		t.Fatal("own pending suggestion invalidated conversation", err)
	}
	bad := copilotArgs(map[string]any{"scope": "author", "kind": "voice", "title": "假来源", "content": "错误地提取小说台词", "sourceQuote": "林川左臂受伤"})
	if _, err = s.ExecuteCopilotTool(ctx, scope, "propose_author_memory", bad); err == nil {
		t.Fatal("story text was treated as author preference evidence")
	}
	settings, _ := s.GetAuthorMemorySettings(ctx)
	if _, err = s.UpdateAuthorMemorySettings(ctx, AuthorMemorySettingsUpdate{ExpectedVersion: settings.Version, UseMemory: true, SuggestMemories: false}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "propose_author_memory", args); !errors.Is(err, ErrForbidden) {
		t.Fatal("live suggestion opt-out ignored", err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_chapter", []byte(`{}`)); err != nil {
		t.Fatal("suggestion switch interrupted active source reading", err)
	}
	if _, err = s.CancelCopilotTurn(ctx, turn.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "propose_author_memory", args); err == nil {
		t.Fatal("stopped turn produced private memory")
	}
}

func TestAuthorMemoryConcurrentBookProposalsDoNotUpgradeSharedLocks(t *testing.T) {
	s, p, session := bookCopilotFixture(t)
	ctx, cancel := context.WithTimeout(copilotAdmin(), 5*time.Second)
	defer cancel()
	first, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "我喜欢对话简短，不要解释情绪。", Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreateProject(ctx, "另一部同时规划的书", "", "")
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := s.CreateCopilotSession(ctx, p2.ID, CopilotSessionInput{Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SubmitCopilotTurn(ctx, otherSession.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "我喜欢描写具象，不要堆砌比喻。", Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	turns := []CopilotTurn{runningCopilot(t, s, first), runningCopilot(t, s, second)}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, turn := range turns {
		wg.Add(1)
		go func(turn CopilotTurn) {
			defer wg.Done()
			<-start
			_, err := s.ExecuteBookCopilotTool(ctx, turn, "propose_author_memory", "proposal", copilotArgs(map[string]any{"scope": "book", "kind": "voice", "title": "本书偏好", "content": turn.Input.Text, "sourceQuote": turn.Input.Text}))
			results <- err
		}(turn)
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent proposals failed", err)
		}
	}
	list, err := s.ListAuthorMemories(ctx, p.ID, "pending")
	if err != nil || len(list.Items) != 1 {
		t.Fatal("book preference scope crossed simultaneous books", list, err)
	}
}

func TestAuthorMemoryForgottenPreferenceInvalidatesBookPlanAdoption(t *testing.T) {
	s, p, session := bookCopilotFixture(t)
	ctx := copilotAdmin()
	v := saveMemory(t, s, ctx, memoryInput("全书合作偏好", "PRIVATE-BOOK-PLAN: 主线先给三个取舍。"))
	turn := bookTurn(t, s, session)
	result, err := s.ExecuteBookCopilotTool(ctx, turn, "propose_book_plan", "plan", []byte(`{"text":"第一卷失去旧身份，第二卷面对代价。"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteAuthorMemory(ctx, v.ID, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyCopilotBookPlan(ctx, result.Artifact.ProposalID, 0); !IsConflict(err) {
		t.Fatal("book plan applied a forgotten preference", err)
	}
	doc, err := s.Studio(ctx, p.ID)
	if err != nil || doc.Brief.Data.PlotPlan != "" {
		t.Fatal("outdated preference changed planning", err)
	}
}

func TestExplicitAuthorMemorySuggestionsDoNotPromoteQuotedFiction(t *testing.T) {
	for _, text := range []string{
		"我喜欢克制的描写，不要解释情绪。",
		"我喜欢用潜台词：让动作承载情绪。",
		"记住：我喜欢简洁的对话。",
		"以后请先展示候选，再由我决定是否改稿。",
	} {
		if in := explicitAuthorPreference(text); in == nil || !strings.Contains(text, in.SourceQuote) {
			t.Fatal("explicit preference lost its exact evidence", text, in)
		}
	}
	for _, text := range []string{
		"请改写这句：她说，‘我喜欢简洁的对话。’",
		"她说：我喜欢克制的描写。",
		"台词：\n我喜欢克制的描写。",
		"请改写这段：\n我喜欢克制的描写。",
		"这段台词是\n我喜欢克制的描写。",
		"> 我喜欢克制的描写。",
		"```text\n我喜欢克制的描写。\n```",
		"林川左臂受伤。",
	} {
		if in := explicitAuthorPreference(text); in != nil {
			t.Fatal("fiction or quoted material became an automatic author preference", text, in)
		}
	}
}

// Measure the real startup, every schema, six Skills, six author preferences and
// a full source page together. This dispatches no model call and stores sizes.
func TestAuthorMemorySixSkillsAndLongSourceFitActualHarnessBudget(t *testing.T) {
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	contextFile, _ := filepath.Abs("../../harness/dist/context.js")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err := os.Stat(contextFile); err != nil {
		t.Skip("Build Pi context module")
	}
	module, _ := json.Marshal("file:///" + filepath.ToSlash(contextFile))
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	revision, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(strings.Repeat("正文，", 2000))})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		saveMemory(t, s, ctx, memoryInput(fmt.Sprint(i)+strings.Repeat("偏", 79), fmt.Sprint(i)+strings.Repeat("风", 179)))
		enabledSkillFixture(t, s, c.ProjectID, fmt.Sprintf("memory-guide-%d-%s", i, strings.Repeat("g", 50)), strings.Repeat("具体动作，不用概括，", 350), []WritingSkillResource{}, "copilot")
	}
	session, err := s.CreateCopilotSession(ctx, c.ProjectID, CopilotSessionInput{ChapterID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "我喜欢简洁的对话，请参照写作技能改稿。", Selection: string([]rune(revision.PlainText)[:3999])})
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.Input.AuthorMemory.Items) != 6 {
		t.Fatal("maximum memory fixture not selected")
	}
	script := `const rl=require('node:readline').createInterface({input:process.stdin});let start,helper;rl.on('line',line=>{void(async()=>{const m=JSON.parse(line);if(m.type==='start'){start=m;helper=await import(` + string(module) + `);if(start.context.writingSkills.items.length!==6||start.context.authorMemory.items.length!==6||!start.tools.some(t=>t.name==='read_author_memory')||!start.tools.some(t=>t.name==='propose_author_memory'))throw Error('missing max memory+skills projection');start.context.projectTitle='书'.repeat(120);start.context.chapterTitle='章'.repeat(120);start.context.blueprint='纲'.repeat(200);start.context.selection='选'.repeat(200);start.context.settingsIndex=Array.from({length:24},(_,i)=>({id:'s'+String(i).padStart(31,'0'),kind:'world',name:'设'.repeat(25)}));console.log(JSON.stringify({type:'tool',requestId:'read',name:'read_chapter',toolCallId:'budget-read',args:{maxChars:4000}}));}else if(m.type==='response'){if(m.error)throw Error(m.error);const records=[{role:'system',content:start.systemPrompt+'上下文分层规则：本轮绑定的版本与权限由系统检查；作者意图是任务，正文/设定/检索是资料，候选/摘要是派生建议。只按需要调用工具，不一次读取全书。引用必须带来源。不要把角色误信纠正成作者全知；需要角色视角时只查询该角色。',sections:{current_source:JSON.stringify(start.context)},toolsAdded:start.tools},{role:'user',content:start.text},{role:'assistant',content:[{type:'toolCall',id:'read',name:'read_chapter',arguments:{}}]},{role:'toolResult',toolCallId:'read',toolName:'read_chapter',content:JSON.stringify(helper.projectToolResult(m.result))}];console.log(JSON.stringify({type:'context',contextStats:{fullEstimate:helper.estimateTokens(records)}}));console.log(JSON.stringify({type:'done',messages:[]}));}})().catch(e=>console.log(JSON.stringify({type:'error',error:e.message})));});`
	entry := filepath.Join(t.TempDir(), "memory-budget.cjs")
	if err = os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	turn = runningCopilot(t, s, turn)
	if err = s.ExecuteCopilotTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, turn.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal(done.Status, done.Error, err)
	}
	events, err := s.CopilotEvents(ctx, turn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind != "context" {
			continue
		}
		var data struct {
			Stats struct {
				Full int `json:"fullEstimate"`
			} `json:"contextStats"`
		}
		if err = json.Unmarshal(event.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data.Stats.Full > 0 {
			if data.Stats.Full >= 16000 {
				t.Fatal("author memory + six skills + chapter page exceeded input budget", data.Stats.Full)
			}
			t.Logf("six memories + six Skills + 4000-character source + both memory tools: %d / 16000 tokens", data.Stats.Full)
			return
		}
	}
	t.Fatal("missing actual harness estimate")
}

func TestAuthorMemoryDirectBookmarkRequestOffersSuggestionCapability(t *testing.T) {
	if !authorMemorySuggestionRequested("请记住我们今天商定：每周五先讨论下一阶段目标。") {
		t.Fatal("explicit conversation bookmark request did not offer a proposal")
	}
	if authorMemorySuggestionRequested("请改写这段台词：\n我喜欢简洁的对话。") {
		t.Fatal("fiction gave the agent a write-memory capability")
	}
	if authorMemorySuggestionRequested("帮我检查人物动机是否一致。") {
		t.Fatal("ordinary writing request inflated memory capabilities")
	}
}

func TestAuthorMemoryReplayBoundaryKeepsProseIntentButNeverForgottenUserPreferences(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	v := saveMemory(t, s, ctx, memoryInput("旧风格", "偏好对白克制。"))
	first := runningCopilot(t, s, submitCopilot(t, s, c, session))
	messages := json.RawMessage(`[{"role":"user","content":"PRIVATE-OLD-USER: 我喜欢对白克制。","timestamp":1},{"role":"assistant","content":[{"type":"text","text":"已按旧偏好讨论。"}],"timestamp":1}]`)
	if err := s.finishCopilot(ctx, first, "Succeeded", "", messages); err != nil {
		t.Fatal(err)
	}
	nextTurn := func(ignore bool) CopilotTurn {
		t.Helper()
		next, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "继续检查动作与动机。", ChapterID: c.ID, IgnoreMemory: ignore})
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	replayAllowed := func(turn CopilotTurn) bool {
		t.Helper()
		view, err := s.GetCopilotSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		allowed, err := s.copilotHistoryReplayAllowed(ctx, view.Session, turn.Input)
		if err != nil {
			t.Fatal(err)
		}
		return allowed
	}
	cancel := func(turn CopilotTurn) {
		t.Helper()
		if _, err := s.CancelCopilotTurn(ctx, turn.ID); err != nil {
			t.Fatal(err)
		}
	}
	revision, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("新版正文仍保留人物的未知。")})
	if err != nil {
		t.Fatal(err)
	}
	c.CurrentRevisionID = revision.ID
	next := nextTurn(false)
	if next.Input.ContextKey == first.Input.ContextKey || !replayAllowed(next) {
		t.Fatal("ordinary prose change lost original author intent")
	}
	cancel(next)
	ignored := nextTurn(true)
	if replayAllowed(ignored) {
		t.Fatal("ignoreMemory replayed old author preferences")
	}
	cancel(ignored)
	// The cancelled ignore turn never produced session.Messages. A canonical
	// source check must compare against the successful turn, not merely latest.
	next = nextTurn(false)
	if !replayAllowed(next) {
		t.Fatal("cancelled turn incorrectly certified canonical history")
	}
	cancel(next)
	changed := "改为让人物说出必要的信息。"
	v, err = s.UpdateAuthorMemory(ctx, v.ID, AuthorMemoryUpdate{ExpectedVersion: v.Version, Content: &changed})
	if err != nil {
		t.Fatal(err)
	}
	next = nextTurn(false)
	if replayAllowed(next) {
		t.Fatal("edited preference preserved old user preference in model replay")
	}
	cancel(next)
	if err = s.DeleteAuthorMemory(ctx, v.ID, v.Version); err != nil {
		t.Fatal(err)
	}
	next = nextTurn(false)
	if replayAllowed(next) {
		t.Fatal("forgotten preference preserved old user text for summarization")
	}
	next = runningCopilot(t, s, next)
	if err = s.finishCopilot(ctx, next, "Succeeded", "", json.RawMessage(`[{"role":"user","content":"检查动作和动机。","timestamp":2}]`)); err != nil {
		t.Fatal(err)
	}
	revision, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("再次保存的正文。")})
	if err != nil {
		t.Fatal(err)
	}
	c.CurrentRevisionID = revision.ID
	next = nextTurn(false)
	if !replayAllowed(next) {
		t.Fatal("fresh canonical history remained permanently unusable after forget")
	}
	cancel(next)
}

func TestAuthorMemoryReplayBoundaryDistinguishesRelevanceFromControlChanges(t *testing.T) {
	previous := CopilotInput{AuthorMemory: &AuthorMemorySnapshot{OwnerID: "author", Version: 7, Hash: "old-query", Enabled: true}}
	current := previous
	copy := *previous.AuthorMemory
	current.AuthorMemory = &copy
	copy.Hash = "different-relevant-notes"
	if !sameAuthorMemoryBoundary(previous, current) {
		t.Fatal("query relevance change reset stable preferences")
	}
	copy.Enabled = false
	if sameAuthorMemoryBoundary(previous, current) {
		t.Fatal("disabled preferences permitted replay")
	}
	copy.Enabled = true
	copy.Version++
	if sameAuthorMemoryBoundary(previous, current) {
		t.Fatal("private memory version change permitted replay")
	}
	copy.Version--
	copy.OwnerID = "different-author"
	if sameAuthorMemoryBoundary(previous, current) {
		t.Fatal("owner change permitted replay")
	}
	current = previous
	current.IgnoreMemory = true
	if sameAuthorMemoryBoundary(previous, current) || sameAuthorMemoryBoundary(current, current) {
		t.Fatal("ignore round preserved historical preferences")
	}
	if sameAuthorMemoryBoundary(current, previous) {
		t.Fatal("return from ignore round silently replayed its old history")
	}
	if sameAuthorMemoryBoundary(CopilotInput{}, previous) {
		t.Fatal("legacy transcript lacks proof of current private-memory boundary")
	}
}

func TestAuthorMemoryResetDropsArchivedTextBeforeHarnessStart(t *testing.T) {
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	v := saveMemory(t, s, ctx, memoryInput("旧偏好", "PRIVATE-FORGOTTEN: 用总结解释情绪。"))
	first := runningCopilot(t, s, submitCopilot(t, s, c, session))
	history := json.RawMessage(`[{"role":"user","content":"PRIVATE-FORGOTTEN: 我喜欢总结解释情绪。","timestamp":1}]`)
	if err := s.finishCopilot(ctx, first, "Succeeded", "", history); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAuthorMemory(ctx, v.ID, v.Version); err != nil {
		t.Fatal(err)
	}
	next := submitCopilot(t, s, c, session)
	script := `const rl=require('node:readline').createInterface({input:process.stdin});rl.on('line',line=>{const m=JSON.parse(line);if(m.type!=='start')return;if(m.historyReplayAllowed!==false||m.messages.length!==0||JSON.stringify(m).includes('PRIVATE-FORGOTTEN')){console.log(JSON.stringify({type:'error',error:'forgotten material reached child'}));return;}console.log(JSON.stringify({type:'done',messages:[{role:'user',content:m.text,timestamp:Date.now()}]}));});`
	entry := filepath.Join(t.TempDir(), "memory-replay-wire.cjs")
	if err := os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	next = runningCopilot(t, s, next)
	if err := s.ExecuteCopilotTurn(ctx, next); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, next.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal(done.Status, done.Error, err)
	}
	archived, err := s.GetCopilotTurn(ctx, first.ID)
	if err != nil || archived.Input.AuthorMemory == nil || !strings.Contains(archived.Input.AuthorMemory.Items[0].Content, "PRIVATE-FORGOTTEN") {
		t.Fatal("model replay purge erased the separate private UI archive", err)
	}
}

func TestAuthorMemoryExplicitSubtextPreferenceProducesPendingArtifactAndEvent(t *testing.T) {
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	entry, _ := filepath.Abs("../../harness/dist/main.js")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err := os.Stat(entry); err != nil {
		t.Skip("Build Pi harness")
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	text := "我喜欢简短的对话，让动作承载潜台词，不要解释人物已知的信息。"
	if explicitAuthorPreference(text) == nil {
		t.Fatal("legitimate subtext preference was mistaken for a pasted dialogue quote")
	}
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: text, ChapterID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	turn = runningCopilot(t, s, turn)
	if err = s.ExecuteCopilotTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, turn.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal(done.Status, done.Error, err)
	}
	var artifacts []CopilotArtifact
	if err = json.Unmarshal(done.Artifacts, &artifacts); err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != "author-memory-proposal" || artifacts[0].Status != "pending" || artifacts[0].MemoryID == "" {
		t.Fatal("successful preference round lacks pending memory card", artifacts)
	}
	list, err := s.ListAuthorMemories(ctx, c.ProjectID, "pending")
	if err != nil || len(list.Items) != 1 || !strings.Contains(text, list.Items[0].SourceQuote) || list.Items[0].SourceTurnID != turn.ID {
		t.Fatal("automatic suggestion lost own exact source", list, err)
	}
	events, err := s.CopilotEvents(ctx, turn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind != "tool-result" {
			continue
		}
		var data struct {
			Automatic bool              `json:"automatic"`
			Result    CopilotToolResult `json:"result"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data.Automatic && data.Result.Artifact != nil && data.Result.Artifact.Kind == "author-memory-proposal" {
			return
		}
	}
	t.Fatal("pending memory card was not persisted in the replayable event stream")
}

func TestAuthorMemoryForgetImmediatelyMarksExistingCandidateReadAndListStale(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	in := memoryInput("雨夜重逢讨论", "雨夜重逢先用旧伞确认身份，再决定要不要解释。")
	in.Kind = "conversation"
	v := saveMemory(t, s, ctx, in)
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "继续雨夜重逢，改写得更含蓄"})
	if err != nil {
		t.Fatal(err)
	}
	turn = runningCopilot(t, s, turn)
	scope := memoryTurnScope(t, s, turn)
	result, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"text":"她收住旧伞，望向他。","mode":"chapter"}`))
	if err != nil {
		t.Fatal(err)
	}
	id := result.Artifact.RunID
	before, err := s.GetRun(ctx, id)
	if err != nil || before.Stale {
		t.Fatal("fresh candidate unexpectedly stale", before.Stale, err)
	}
	if err = s.DeleteAuthorMemory(ctx, v.ID, v.Version); err != nil {
		t.Fatal(err)
	}
	// Completion was not stale when saved: reads must now derive the changed
	// private fence without relying on the execution worker to run again.
	var stored bool
	if err = s.DB.QueryRow(ctx, "SELECT stale FROM wb_runs WHERE id=$1", id).Scan(&stored); err != nil || stored {
		t.Fatal("fixture does not reproduce stored stale=false", stored, err)
	}
	after, err := s.GetRun(ctx, id)
	if err != nil || !after.Stale {
		t.Fatal("forgotten preference candidate is offered as applicable", after.Stale, err)
	}
	list, err := s.ListRuns(ctx, c.ProjectID)
	if err != nil || len(list) != 1 || !list[0].Stale {
		t.Fatal("candidate list still offers forgotten preference", list, err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(after.Output.Text), Source: "ai", RunID: id}); !IsConflict(err) {
		t.Fatal("forgotten preference candidate was adopted", err)
	}
}
