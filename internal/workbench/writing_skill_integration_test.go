package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"novel/internal/job"
	"novel/internal/llm"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func enabledSkillFixture(t *testing.T, s *Service, pid, name, body string, resources []WritingSkillResource, tasks ...string) WritingSkill {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: Use concrete actions and imperfect dialogue\n---\n" + body
	skill, err := s.SaveWritingSkill(copilotAdmin(), pid, WritingSkillSave{Preview: SkillPreview{Content: content, Resources: resources}, Enabled: true, TaskTypes: tasks})
	if err != nil {
		t.Fatal(err)
	}
	return skill
}
func TestWritingSkillWholeMainInstructionsReachActualGenerationWithoutResourceLeak(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	skill := enabledSkillFixture(t, s, c.ProjectID, "concrete-actions", "保留角色迟疑，用手指的动作表达情绪；不要用总结句替人物解释。", []WritingSkillResource{{Path: "references/dialogue.md", Content: "尚未读取的专属资料标记"}}, "draft", "planning", "review", "rewrite")
	calls := 0
	s.Provider = &llm.Fake{Handler: func(ctx context.Context, request llm.Request) (llm.Response, error) {
		calls++
		raw, _ := json.Marshal(request.Messages)
		text := string(raw)
		if !strings.Contains(text, "用手指的动作表达情绪") || !strings.Contains(text, "写作方法与审美参考") || !strings.Contains(text, "references/dialogue.md") {
			t.Error("enabled complete instructions missing from actual provider request")
		}
		if strings.Contains(text, "尚未读取的专属资料标记") {
			t.Error("reference body silently hitchhiked into one-shot context")
		}
		return llm.Response{Text: "林川的手指停在铜印边缘。", Raw: []byte("local provider response")}, nil
	}}
	run := submit(t, s, c, "draft")
	if run.Input.WritingSkills == nil || run.Input.WritingSkills.Version == 0 || len(run.Input.WritingSkills.Items) != 1 || run.Input.WritingSkills.Items[0].ID != skill.ID || run.Input.WritingSkills.Items[0].Resources[0].Content != "尚未读取的专属资料标记" {
		t.Fatal("audit lost frozen skill source")
	}
	if err := s.ExecuteRun(ctx, claim(t, s)); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetRun(ctx, run.ID)
	if err != nil || done.Status != "Succeeded" || calls != 1 {
		t.Fatal("skill-bearing generation failed", done.Status, err)
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil || current.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("generation changed author source", err)
	}
	for _, request := range []RunRequest{{TaskType: "planning"}, {TaskType: "review"}, {TaskType: "rewrite", Selection: "林川左臂受伤。", Operation: "polish"}} {
		request.IntentID = job.NewID()
		request.InputRevisionID = c.CurrentRevisionID
		run, err := s.SubmitRun(ctx, c.ID, request)
		if err != nil || !strings.Contains(run.Input.Request.Messages[1].Content, "用手指的动作表达情绪") {
			t.Fatal("task omitted skills", request.TaskType, err)
		}
	}
}
func TestWritingSkillOversizeLegacyInstructionsRejectBeforeEnqueueButCopilotCanRead(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	skill := enabledSkillFixture(t, s, c.ProjectID, "long-writing-guide", strings.Repeat("长规则", 2100), []WritingSkillResource{})
	if _, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "draft"}); err == nil || !strings.Contains(err.Error(), "6000") {
		t.Fatal("oversize instruction silently cut", err)
	}
	var count int
	if err := s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_runs").Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected oversized task was enqueued", count, err)
	}
	scope := copilotScope(t, s, c, "")
	got, err := s.ExecuteCopilotTool(ctx, scope, "read_writing_skill", copilotArgs(map[string]any{"skillId": skill.ID, "offset": 6000, "maxChars": 1000}))
	if err != nil || got.Content.(map[string]any)["totalChars"].(int) < 6000 {
		t.Fatal("long skill not progressively readable", err)
	}
}
func TestWritingSkillVersionBlocksQueuedDispatchAndPreSkillCandidateAdoption(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	oldScope := copilotScope(t, s, c, "")
	result, err := s.ExecuteCopilotTool(ctx, oldScope, "create_revision_candidate", copilotArgs(map[string]string{"text": "旧技能基线候选", "mode": "chapter"}))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a genuine historic artifact before this feature existed.
	if _, err = s.DB.Exec(ctx, "UPDATE wb_runs SET input=input-'writingSkills' WHERE id=$1", result.Artifact.RunID); err != nil {
		t.Fatal(err)
	}
	skill := enabledSkillFixture(t, s, c.ProjectID, "current-style", "保留停顿，不在段尾总结", nil, "draft", "copilot")
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(result.Artifact.Text), Source: "ai", RunID: result.Artifact.RunID}); !IsConflict(err) {
		t.Fatal("pre-skill candidate adopted against new instructions", err)
	}
	queued := submit(t, s, c, "draft")
	claimed := claim(t, s)
	disabled := false
	if _, err = s.UpdateWritingSkill(ctx, c.ProjectID, skill.ID, WritingSkillUpdate{ExpectedVersion: skill.Version, Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		return llm.Response{Text: "must not call"}, nil
	}}
	if err = s.ExecuteRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetRun(ctx, queued.ID)
	if err != nil || done.Status == "Succeeded" || calls != 0 {
		t.Fatal("outdated skill dispatched", done.Status, calls, err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, oldScope, "read_chapter", []byte("{}")); !IsConflict(err) {
		t.Fatal("old no-skill scope remained live", err)
	}
}
func TestWritingSkillBookPlanAndContextKeyRejectChangedSkillVersion(t *testing.T) {
	s, p, session := bookCopilotFixture(t)
	ctx := copilotAdmin()
	skill := enabledSkillFixture(t, s, p.ID, "book-dialogue", "用选择的代价推进，不自动添加世界事实。", nil, "copilot")
	turn := bookTurn(t, s, session)
	if turn.Input.WritingSkills == nil || len(turn.Input.WritingSkills.Items) != 1 {
		t.Fatal("book turn did not freeze selected skills")
	}
	read, err := s.ExecuteBookCopilotTool(ctx, turn, "read_writing_skill", "read-skill", copilotArgs(map[string]string{"skillId": skill.ID}))
	if err != nil || !strings.Contains(read.Content.(map[string]any)["text"].(string), "选择的代价") {
		t.Fatal("book skill reader failed", err)
	}
	plan, err := s.ExecuteBookCopilotTool(ctx, turn, "propose_book_plan", "plan", []byte("{\"text\":\"全书候选计划\"}"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage("[]")); err != nil {
		t.Fatal(err)
	}
	newBody := "---\nname: book-dialogue\ndescription: New approach\n---\n关注人物行动的后果，不复制通用模板。"
	if _, err = s.UpdateWritingSkill(ctx, p.ID, skill.ID, WritingSkillUpdate{ExpectedVersion: skill.Version, Content: &newBody}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyCopilotBookPlan(ctx, plan.Artifact.ProposalID, 0); !IsConflict(err) {
		t.Fatal("book plan adopted under different skill version", err)
	}
	stale := runningCopilot(t, s, turn)
	if _, err = s.ExecuteBookCopilotTool(ctx, stale, "read_writing_skill", "old-read", copilotArgs(map[string]string{"skillId": skill.ID})); !IsConflict(err) {
		t.Fatal("running stale book source can read skill", err)
	}
	if err = s.finishCopilot(ctx, stale, "Failed", "input changed", nil); err != nil {
		t.Fatal(err)
	}
	next, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "继续刚才规划"})
	if err != nil || next.Input.ContextKey == turn.Input.ContextKey {
		t.Fatal("skill change failed to rebase conversation", err)
	}
}
func TestWritingSkillFrozenReaderPreservesEveryCharacterAndDoesNotAuthorizeOtherFiles(t *testing.T) {
	original := strings.Repeat("具体动作，", 1400) + "最后规则"
	reference := strings.Repeat("人物说一半，", 800) + "末尾资料"
	snapshot := &WritingSkillSnapshot{Version: 7, Hash: "frozen-set", Items: []WritingSkillInstruction{{ID: "s1", Name: "dialogue", Version: 3, Hash: "source-hash", Content: original, Resources: []WritingSkillResource{{Path: "references/dialogue.md", Content: reference}}}}}
	for _, entry := range []struct{ path, text string }{{"SKILL.md", original}, {"references/dialogue.md", reference}} {
		var all strings.Builder
		for at := 0; at < len([]rune(entry.text)); {
			result, err := readFrozenWritingSkill(snapshot, copilotArgs(map[string]any{"skillId": "s1", "path": entry.path, "offset": at, "maxChars": 700}))
			if err != nil {
				t.Fatal(err)
			}
			page := result.Content.(map[string]any)
			text := page["text"].(string)
			all.WriteString(text)
			next := page["nextOffset"].(int)
			if next != at+len([]rune(text)) {
				t.Fatal("source cursor skipped characters")
			}
			at = next
			if page["version"] != int64(3) || page["hash"] != "source-hash" {
				t.Fatal("resource provenance missing")
			}
		}
		if all.String() != entry.text {
			t.Fatal("partial source misrepresented as full")
		}
	}
	for _, args := range []map[string]any{{"skillId": "other-book"}, {"skillId": "s1", "path": "../../.env"}, {"skillId": "s1", "path": "scripts/write.ps1"}} {
		if _, err := readFrozenWritingSkill(snapshot, copilotArgs(args)); !errors.Is(err, ErrNotFound) {
			t.Fatal("reader accepted unrelated material", err)
		}
	}
	raw, _ := json.Marshal(writingSkillCopilotContext(snapshot))
	if strings.Contains(string(raw), "最后规则") || strings.Contains(string(raw), "末尾资料") || !strings.Contains(string(raw), "truncated") {
		t.Fatal("startup not bounded/explicit")
	}
}
func TestWritingSkillActualPiUsesFrozenSkillBeforeBookPlan(t *testing.T) {
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
	s, p, session := bookCopilotFixture(t)
	ctx := copilotAdmin()
	skill := enabledSkillFixture(t, s, p.ID, "writer-rhythm", "保留必要的不完整表达和人物各自的语速。", nil, "copilot")
	turn := bookTurn(t, s, session)
	if err := s.ExecuteCopilotTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, turn.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal("Pi skill chain failed", done.Status, done.Error, err)
	}
	events, err := s.CopilotEvents(ctx, turn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	readSkill, readBook, plan := -1, -1, -1
	for i, event := range events {
		if event.Kind != "tool-result" {
			continue
		}
		var eventData struct {
			Name   string            `json:"name"`
			Result CopilotToolResult `json:"result"`
		}
		if err = json.Unmarshal(event.Data, &eventData); err != nil {
			t.Fatal(err)
		}
		switch eventData.Name {
		case "read_writing_skill":
			readSkill = i
			raw, _ := json.Marshal(eventData.Result.Content)
			if !strings.Contains(string(raw), skill.ID) || !strings.Contains(string(raw), "人物各自的语速") {
				t.Fatal("Pi did not receive frozen skill source")
			}
		case "read_book_brief":
			readBook = i
		case "propose_book_plan":
			plan = i
		}
	}
	if readSkill < 0 || readBook < readSkill || plan < readBook {
		t.Fatal("Pi skipped skill or wrong tool ordering", readSkill, readBook, plan)
	}
	before, err := s.Studio(ctx, p.ID)
	if err != nil || before.Brief.Data.PlotPlan != "" {
		t.Fatal("skill-driven agent adopted plan automatically", err)
	}
}

func TestWritingSkillStyleChangeDoesNotInvalidateFinalizedMemoryEvidence(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := copilotAdmin()
	c := f.chapter(t, "记忆来源", "林川左臂受伤。")
	change := f.change(t, c, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂受伤")
	data, _ := json.Marshal(map[string]any{"changes": []MemoryChange{change}})
	f.s.Model, f.s.ProviderName = "fake-v1", "fake"
	calls := 0
	f.s.Provider = &llm.Fake{Handler: func(_ context.Context, request llm.Request) (llm.Response, error) {
		calls++
		raw, _ := json.Marshal(request.Messages)
		if strings.Contains(string(raw), "不要让文风成为事实") {
			t.Error("creative skill leaked into evidence extractor")
		}
		return llm.Response{Text: string(data), Raw: data}, nil
	}}
	run, err := f.s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "memory-extraction"})
	if err != nil {
		t.Fatal(err)
	}
	enabledSkillFixture(t, f.s, f.p.ID, "evidence-independent", "不要让文风成为事实", nil)
	claimed, ok, err := f.s.ClaimRun(ctx, "skill-memory-test")
	if err != nil || !ok || claimed.ID != run.ID {
		t.Fatal(ok, err)
	}
	if err = f.s.ExecuteRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	done, err := f.s.GetRun(ctx, run.ID)
	if err != nil || done.Status != "Succeeded" || done.Stale || done.Output.ProposalID == "" || calls != 1 {
		t.Fatal("style invalidated source evidence", done.Status, calls, err)
	}
	proposal, err := f.s.GetProposal(ctx, done.Output.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	state := f.apply(t, proposal)
	if len(state.Facts) != 1 {
		t.Fatal("evidence proposal was not applicable after style change")
	}
}

func TestWritingSkillStartupAndSingleSourceFitActualHarnessBudget(t *testing.T) {
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	contextFile, _ := filepath.Abs("../../harness/dist/context.js")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err := os.Stat(contextFile); err != nil {
		t.Skip("Build Pi context module")
	}
	module, _ := json.Marshal("file:///" + filepath.ToSlash(contextFile))
	for _, tool := range []string{"read_chapter", "read_writing_skill"} {
		t.Run(tool, func(t *testing.T) {
			s, c := runtimeFixture(t)
			ctx := copilotAdmin()
			revision, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(strings.Repeat("正文，", 2000))})
			if err != nil {
				t.Fatal(err)
			}
			resources := []WritingSkillResource{}
			for i := 0; i < 40; i++ {
				resources = append(resources, WritingSkillResource{Path: fmt.Sprintf("references/%02d-%s.md", i, strings.Repeat("r", 150)), Content: strings.Repeat("不应隐式加载资料", 100)})
			}
			for i := 0; i < 6; i++ {
				enabledSkillFixture(t, s, c.ProjectID, fmt.Sprintf("guide-%d-%s", i, strings.Repeat("g", 60)), strings.Repeat("具体动作，不用概括，", 350), resources, "copilot")
			}
			session, err := s.CreateCopilotSession(ctx, c.ProjectID, CopilotSessionInput{ChapterID: c.ID})
			if err != nil {
				t.Fatal(err)
			}
			turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "参照写作技能润色当前选段", Selection: string([]rune(revision.PlainText)[:3999])})
			if err != nil {
				t.Fatal(err)
			}
			name, _ := json.Marshal(tool)
			script := `const rl=require('node:readline').createInterface({input:process.stdin});let start,helper;rl.on('line',line=>{void(async()=>{const m=JSON.parse(line);if(m.type==='start'){start=m;helper=await import(` + string(module) + `);if(start.context.writingSkills.items.length!==6||start.context.writingSkills.items.some(s=>!s.name||!s.description)||!start.tools.some(t=>t.name==='read_writing_skill')||JSON.stringify(start.context).includes('不应隐式加载资料'))throw Error('skill context not projected');start.context.projectTitle='书'.repeat(120);start.context.chapterTitle='章'.repeat(120);start.context.blueprint='纲'.repeat(200);start.context.selection='选'.repeat(200);start.context.settingsIndex=Array.from({length:24},(_,i)=>({id:'s'+String(i).padStart(31,'0'),kind:'world',name:'设'.repeat(25)}));console.log(JSON.stringify({type:'tool',requestId:'read',name:` + string(name) + `,toolCallId:'budget-read',args:` + string(name) + `==='read_writing_skill'?{skillId:start.context.writingSkills.items[0].id,maxChars:2000}:{maxChars:4000}}));}else if(m.type==='response'){if(m.error)throw Error(m.error);const projected=helper.projectToolResult(m.result);const records=[{role:'system',content:start.systemPrompt+'上下文分层规则：本轮绑定的版本与权限由系统检查；作者意图是任务，正文/设定/检索是资料，候选/摘要是派生建议。只按需要调用工具，不一次读取全书。引用必须带来源。不要把角色误信纠正成作者全知；需要角色视角时只查询该角色。',sections:{current_source:JSON.stringify(start.context)},toolsAdded:start.tools},{role:'user',content:start.text},{role:'assistant',content:[{type:'toolCall',id:'read',name:` + string(name) + `,arguments:{}}]},{role:'toolResult',toolCallId:'read',toolName:` + string(name) + `,content:JSON.stringify(projected)}];console.log(JSON.stringify({type:'context',contextStats:{fullEstimate:helper.estimateTokens(records),toolEstimate:helper.estimateTokens(projected)}}));console.log(JSON.stringify({type:'done',messages:[]}));}})().catch(e=>console.log(JSON.stringify({type:'error',error:e.message})));});`
			entry := filepath.Join(t.TempDir(), "skill-estimate.cjs")
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
			found := false
			for _, event := range events {
				if event.Kind == "context" {
					var data struct {
						Stats struct {
							Full int `json:"fullEstimate"`
							Tool int `json:"toolEstimate"`
						} `json:"contextStats"`
					}
					if err = json.Unmarshal(event.Data, &data); err != nil {
						t.Fatal(err)
					}
					if data.Stats.Full > 0 {
						found = true
						if data.Stats.Full >= 16000 {
							t.Fatalf("single skill/source exceeded input budget: full=%d tool=%d", data.Stats.Full, data.Stats.Tool)
						}
						t.Logf("six active skills, actual startup + tool schemas + %s: full=%d tool=%d, budget=16000", tool, data.Stats.Full, data.Stats.Tool)
					}
				}
			}
			if !found {
				t.Fatal("missing actual harness context estimate")
			}
		})
	}
}

func TestWritingSkillCandidateAdoptionRequiresSameSkillAndBlueprintSnapshot(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	skill := enabledSkillFixture(t, s, c.ProjectID, "adopt-style", "只依据已读取来源创作，保留作者的节奏。", nil, "copilot")
	scope := copilotScope(t, s, c, "")
	result, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", copilotArgs(map[string]string{"text": "林川把铜印移开，手停在桌沿。", "mode": "chapter"}))
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.GetRun(ctx, result.Artifact.RunID)
	if err != nil || run.Input.WritingSkills == nil || run.Input.WritingSkills.Version != scope.WritingSkills.Version || run.Input.WritingSkills.Hash != scope.WritingSkills.Hash {
		t.Fatal("candidate lost skill baseline", err)
	}
	adopted, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(result.Artifact.Text), Source: "ai", RunID: result.Artifact.RunID})
	if err != nil {
		t.Fatal("unchanged enabled skill blocked valid candidate", err)
	}
	c.CurrentRevisionID = adopted.ID
	scope = copilotScope(t, s, c, "")
	proposal, err := s.ExecuteCopilotTool(ctx, scope, "propose_blueprint", copilotArgs(map[string]string{"text": "只提议下一场景的取舍。"}))
	if err != nil {
		t.Fatal(err)
	}
	body := "---\nname: adopt-style\ndescription: revised\n---\n用对话的省略推进。"
	if _, err = s.UpdateWritingSkill(ctx, c.ProjectID, skill.ID, WritingSkillUpdate{ExpectedVersion: skill.Version, Content: &body}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyCopilotBlueprint(ctx, proposal.Artifact.RunID, c.BlueprintVersion); !IsConflict(err) {
		t.Fatal("skill change allowed old blueprint adoption", err)
	}
}

func TestWritingSkillChangeDuringProviderCallMakesCompletionStale(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	skill := enabledSkillFixture(t, s, c.ProjectID, "dispatch-style", "用行动推进。", nil, "draft")
	run := submit(t, s, c, "draft")
	calls := 0
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		enabled := false
		if _, err := s.UpdateWritingSkill(ctx, c.ProjectID, skill.ID, WritingSkillUpdate{ExpectedVersion: skill.Version, Enabled: &enabled}); err != nil {
			return llm.Response{}, err
		}
		return llm.Response{Text: "过期技能生成结果", Raw: []byte("test response")}, nil
	}}
	if err := s.ExecuteRun(ctx, claim(t, s)); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetRun(ctx, run.ID)
	if err != nil || !done.Stale || calls != 1 {
		t.Fatal("skill baseline changed during model call but completion stayed current", done.Status, done.Stale, calls, err)
	}
	if _, err = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(done.Output.Text), Source: "ai", RunID: run.ID}); !IsConflict(err) {
		t.Fatal("stale completion adopted", err)
	}
}

func TestWritingSkillProgressiveCatalogKeepsAllEnabledInstructionsDiscoverable(t *testing.T) {
	snapshot := &WritingSkillSnapshot{Version: 12, Hash: "set-version"}
	for i := 0; i < 6; i++ {
		item := WritingSkillInstruction{ID: fmt.Sprint(i), Name: strings.Repeat("目录名称", 20), Description: strings.Repeat("适用场景", 30), Content: strings.Repeat("写作指引", 500), Version: 1, Hash: fmt.Sprint("hash", i)}
		for r := 0; r < 40; r++ {
			item.Resources = append(item.Resources, WritingSkillResource{Path: fmt.Sprintf("references/%02d.md", r), Content: "参考资料"})
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	raw, _ := json.Marshal(writingSkillCopilotContext(snapshot))
	var projected struct {
		Items []struct{ Name, Description, Preview string }
	}
	if err := json.Unmarshal(raw, &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected.Items) != 6 {
		t.Fatal("enabled skill disappeared from startup")
	}
	names, body := 0, 0
	for _, entry := range projected.Items {
		if entry.Name == "" || entry.Description == "" {
			t.Fatal("later skills became undiscoverable")
		}
		names += len([]rune(entry.Name)) + len([]rune(entry.Description))
		body += len([]rune(entry.Preview))
	}
	if names > 300 || body > 600 {
		t.Fatal("startup budget exceeded", names, body)
	}
	seen := map[string]bool{}
	for offset := 0; ; {
		result, err := readFrozenWritingSkill(snapshot, copilotArgs(map[string]any{"offset": offset}))
		if err != nil {
			t.Fatal(err)
		}
		page := result.Content.(map[string]any)
		for _, entry := range page["items"].([]map[string]any) {
			id := entry["id"].(string)
			if seen[id] {
				t.Fatal("duplicate skill page")
			}
			seen[id] = true
		}
		if page["hasMore"] == false {
			break
		}
		offset = page["nextOffset"].(int)
	}
	if len(seen) != 6 {
		t.Fatal("catalog pagination lost enabled skill")
	}
	paths := map[string]bool{}
	for offset := 0; ; {
		result, err := readFrozenWritingSkill(snapshot, copilotArgs(map[string]any{"skillId": "5", "resourceOffset": offset}))
		if err != nil {
			t.Fatal(err)
		}
		page := result.Content.(map[string]any)
		for _, path := range page["resourcePaths"].([]string) {
			if paths[path] {
				t.Fatal("duplicate resource page")
			}
			paths[path] = true
		}
		if page["resourcesHaveMore"] == false {
			break
		}
		offset = page["nextResourceOffset"].(int)
	}
	if len(paths) != 40 {
		t.Fatal("resource index lost relative document")
	}
}
