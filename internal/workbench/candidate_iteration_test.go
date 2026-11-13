package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel/internal/job"
)

func completedIterationCandidate(t *testing.T, s *Service, c Chapter, session CopilotSession, text string) (Run, CopilotTurn) {
	t.Helper()
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	scope, err := s.NewCopilotToolContext(copilotAdmin(), turn.ID, c.ProjectID, c.ID, c.CurrentRevisionID, "")
	if err != nil {
		t.Fatal(err)
	}
	scope.CallID = "initial-candidate"
	result, err := s.ExecuteCopilotTool(copilotAdmin(), scope, "create_revision_candidate", copilotArgs(map[string]string{"mode": "chapter", "text": text}))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finishCopilot(context.Background(), turn, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	run, err := s.GetRun(context.Background(), result.Artifact.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return run, turn
}

func iterationScope(t *testing.T, s *Service, turn CopilotTurn) CopilotToolContext {
	t.Helper()
	scope, err := s.NewCopilotToolContext(copilotAdmin(), turn.ID, turn.ProjectID, turn.Input.ChapterID, turn.Input.InputRevisionID, turn.Input.Selection)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func joinIterationDiff(hunks []CopilotDiffHunk, before bool) string {
	all := []string{}
	for _, h := range hunks {
		text := h.After
		if before {
			text = h.Before
		}
		if text != "" {
			all = append(all, text)
		}
	}
	return strings.Join(all, "\n\n")
}

func TestCandidateIterationPagesFrozenSelectionPreservesParentAndOriginalAdoption(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	selected := strings.Repeat("她停在门边，", 200) + "候选选段唯一尾。"
	parentText := "上一稿改动：林川用右手把铜印收好。\n\n" + selected + "\n\n上一稿结尾：沈月并未知道他的伤势。"
	parent, _ := completedIterationCandidate(t, s, c, session, parentText)
	next, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "只把这一段改得更克制，保留上一稿其他变化", CandidateRunID: parent.ID, CandidateSelection: selected})
	if err != nil {
		t.Fatal(err)
	}
	if next.Input.Candidate == nil || next.Input.Candidate.RunID != parent.ID || next.Input.Candidate.TextHash != job.Hash([]byte(parentText)) || next.Input.Candidate.TotalChars != len([]rune(parentText)) {
		t.Fatal("parent source was not frozen", next.Input.Candidate)
	}
	next = runningCopilot(t, s, next)
	scope := iterationScope(t, s, next)
	var full strings.Builder
	for offset := 0; offset < len([]rune(selected)); {
		page, e := s.ExecuteCopilotTool(ctx, scope, "read_revision_candidate", copilotArgs(map[string]any{"selection": true, "offset": offset, "maxChars": 200}))
		if e != nil {
			t.Fatal(e)
		}
		value := page.Content.(map[string]any)
		piece := value["text"].(string)
		if value["nextOffset"].(int) != offset+len([]rune(piece)) || value["selectionHash"] != job.Hash([]byte(selected)) || value["candidateHash"] != next.Input.Candidate.TextHash {
			t.Fatal("candidate cursor/identity changed", value)
		}
		full.WriteString(piece)
		offset = value["nextOffset"].(int)
	}
	if full.String() != selected {
		t.Fatal("candidate selected source truncated or skipped")
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_revision_candidate", []byte(`{"runId":"other"}`)); err == nil {
		t.Fatal("model switched bound candidate")
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_revision_candidate", copilotArgs(map[string]any{"offset": len([]rune(parentText)) + 1})); err == nil {
		t.Fatal("out of range page accepted")
	}
	scope.CallID = "iteration-selection"
	result, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", copilotArgs(map[string]string{"mode": "selection", "text": "她把话咽了回去。"}))
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.GetRun(ctx, result.Artifact.RunID)
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Replace(parentText, selected, "她把话咽了回去。", 1)
	if child.Output.Text != expected || child.Input.CandidateRunID != parent.ID || child.Input.RevisionID != c.CurrentRevisionID || result.Artifact.ParentCandidateRunID != parent.ID {
		t.Fatal("iteration lost parent changes, source or lineage", child)
	}
	original, err := s.GetRevision(ctx, c.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if child.Input.Source != original.PlainText {
		t.Fatal("parent candidate replaced source evidence")
	}
	repeated, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", copilotArgs(map[string]string{"mode": "selection", "text": "她把话咽了回去。"}))
	if err != nil || repeated.Artifact.RunID != child.ID {
		t.Fatal("same call replay duplicated iteration", err)
	}
	sourceDiff, err := s.ExecuteCopilotTool(ctx, scope, "show_diff", copilotArgs(map[string]string{"runId": child.ID}))
	if err != nil {
		t.Fatal(err)
	}
	parentDiff, err := s.ExecuteCopilotTool(ctx, scope, "show_diff", copilotArgs(map[string]string{"runId": child.ID, "compareTo": "parent"}))
	if err != nil {
		t.Fatal(err)
	}
	if joinIterationDiff(sourceDiff.Artifact.Diff, true) != original.PlainText || joinIterationDiff(parentDiff.Artifact.Diff, true) != parentText || joinIterationDiff(parentDiff.Artifact.Diff, false) != expected || parentDiff.Artifact.ComparisonRunID != parent.ID || parentDiff.Artifact.ComparisonBase != "parent" {
		t.Fatal("comparison bases not faithful")
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil || current.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("iteration tool changed saved manuscript", err)
	}
	if err = s.finishCopilot(ctx, next, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	third, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "对这个版本继续讨论", CandidateRunID: child.ID})
	if err != nil || third.Input.Candidate.ParentCandidateRunID != parent.ID {
		t.Fatal("multi-version continuation failed", third, err)
	}
	if _, err = s.CancelCopilotTurn(ctx, third.ID); err != nil {
		t.Fatal(err)
	}
	adopted, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(child.Output.Text), Source: "ai", RunID: child.ID})
	if err != nil || adopted.ParentID != original.ID || adopted.PlainText != expected {
		t.Fatal("author could not adopt continued candidate against original", adopted, err)
	}
	if _, err = s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "旧候选不能继续", CandidateRunID: parent.ID}); !IsConflict(err) {
		t.Fatal("adopted old baseline still accepted", err)
	}
}

func TestCandidateIterationRejectsUnfinishedForeignAmbiguousAndBookBindings(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	parent, _ := completedIterationCandidate(t, s, c, session, "重复段。\n\n重复段。\n\n唯一段。")
	for _, req := range []CopilotTurnRequest{
		{Text: "无目标选段", CandidateSelection: "唯一段。"},
		{Text: "歧义选段", CandidateRunID: parent.ID, CandidateSelection: "重复段。"},
		{Text: "原稿和候选同时引用", CandidateRunID: parent.ID, Selection: "林川左臂受伤。"},
		{Text: "整理上下文", CandidateRunID: parent.ID, Mode: "compact"},
	} {
		req.IntentID = job.NewID()
		if _, err := s.SubmitCopilotTurn(ctx, session.ID, req); err == nil {
			t.Fatal("invalid candidate binding accepted", req)
		}
	}
	other, err := s.CreateChapter(ctx, c.ProjectID, "另章", "")
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := s.CreateCopilotSession(ctx, c.ProjectID, CopilotSessionInput{ChapterID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitCopilotTurn(ctx, otherSession.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "跨章候选", CandidateRunID: parent.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign chapter accepted", err)
	}
	otherCtx := WithPrincipal(ctx, Principal{ID: "different-author", Admin: true})
	foreign, err := s.CreateCopilotSession(otherCtx, c.ProjectID, CopilotSessionInput{ChapterID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitCopilotTurn(otherCtx, foreign.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "其他作者继续", CandidateRunID: parent.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign private candidate accepted", err)
	}
	bookSession, err := s.CreateCopilotSession(ctx, c.ProjectID, CopilotSessionInput{Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitCopilotTurn(ctx, bookSession.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "全书候选", CandidateRunID: parent.ID}); err == nil {
		t.Fatal("book bound chapter candidate")
	}
	running := runningCopilot(t, s, submitCopilot(t, s, c, session))
	scope := iterationScope(t, s, running)
	draft, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"text":"尚未完成的候选。","mode":"chapter"}`))
	if err != nil {
		t.Fatal(err)
	}
	separate, err := s.CreateCopilotSession(ctx, c.ProjectID, CopilotSessionInput{ChapterID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitCopilotTurn(ctx, separate.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "不能偷用生成中候选", CandidateRunID: draft.Artifact.RunID}); !IsConflict(err) {
		t.Fatal("unfinished turn candidate accepted", err)
	}
	if _, err = s.CancelCopilotTurn(ctx, running.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SubmitCopilotTurn(ctx, separate.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "不能偷用取消候选", CandidateRunID: draft.Artifact.RunID}); !IsConflict(err) {
		t.Fatal("cancelled source candidate accepted", err)
	}
}

func TestCandidateIterationRevalidatesFrozenParentAtAuthorizationAndCreation(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	parent, _ := completedIterationCandidate(t, s, c, session, "父候选：保持伤势，先到桥边。")
	next, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "继续修改", CandidateRunID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	next = runningCopilot(t, s, next)
	scope := iterationScope(t, s, next)
	// Completed candidates are immutable through public APIs. Even a corrupted
	// stored output cannot silently replace the author's selected target.
	if _, err = s.DB.Exec(ctx, `UPDATE wb_runs SET output=jsonb_set(output,'{text}','"被篡改的候选"') WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.copilotAuthorize(ctx, next, copilotWire{Messages: json.RawMessage(`[]`), MaxOutputTokens: 256, InputTokensEstimate: 10}); !IsConflict(err) {
		t.Fatal("modified parent authorized", err)
	}
	for _, name := range []string{"read_revision_candidate", "create_revision_candidate"} {
		args := json.RawMessage(`{}`)
		if name == "create_revision_candidate" {
			args = []byte(`{"mode":"chapter","text":"非法新候选"}`)
		}
		if _, err = s.ExecuteCopilotTool(ctx, scope, name, args); !IsConflict(err) {
			t.Fatal("frozen parent fence bypassed", name, err)
		}
	}
	var count int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_runs WHERE input->>'candidateRunId'=$1", parent.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid child persisted", count, err)
	}
}

func TestCandidateIterationActualPiReadsTargetAndKeepsSelectionLineage(t *testing.T) {
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
	parent, _ := completedIterationCandidate(t, s, c, session, "上一稿开头：伤势尚未好。\n\n这一段还很直白。\n\n上一稿结尾：秘密仍未公开。")
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "把这段写得更含蓄", CandidateRunID: parent.ID, CandidateSelection: "这一段还很直白。"})
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
	var child *CopilotArtifact
	for i := range artifacts {
		if artifacts[i].Kind == "revision-candidate" {
			child = &artifacts[i]
		}
	}
	if child == nil || child.ParentCandidateRunID != parent.ID || !strings.Contains(child.Text, "上一稿开头") || !strings.Contains(child.Text, "上一稿结尾") || strings.Contains(child.Text, "这一段还很直白") {
		t.Fatal("Pi iteration lost parent edits or target", artifacts)
	}
	events, err := s.CopilotEvents(ctx, turn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, event := range events {
		if event.Kind == "tool-start" {
			var v struct {
				Name string `json:"name"`
			}
			if err = json.Unmarshal(event.Data, &v); err != nil {
				t.Fatal(err)
			}
			names = append(names, v.Name)
		}
	}
	if strings.Join(names, ",") != "read_chapter,read_revision_candidate,create_revision_candidate,show_diff" {
		t.Fatal("Pi did not read bound target", names)
	}
}

func TestCandidateIterationMaximumStartupAndPageFitContextBudget(t *testing.T) {
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	helperPath, _ := filepath.Abs("../../harness/dist/context.js")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err := os.Stat(helperPath); err != nil {
		t.Skip("Build Pi harness")
	}
	module, _ := json.Marshal("file:///" + filepath.ToSlash(helperPath))
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	for i := 0; i < 6; i++ {
		saveMemory(t, s, ctx, memoryInput(fmt.Sprint(i)+strings.Repeat("偏", 79), fmt.Sprint(i)+strings.Repeat("风", 179)))
		enabledSkillFixture(t, s, c.ProjectID, fmt.Sprintf("iteration-%d-%s", i, strings.Repeat("g", 50)), strings.Repeat("具体动作，不用概括，", 350), []WritingSkillResource{}, "copilot")
	}
	if _, err := s.SaveStudioBrief(ctx, c.ProjectID, StudioWrite[StoryBrief]{Data: StoryBrief{Style: strings.Repeat("风格", 500), Pace: "避免重复铺陈", Restrictions: "保留人物知识边界"}}); err != nil {
		t.Fatal(err)
	}
	parent, _ := completedIterationCandidate(t, s, c, session, strings.Repeat("候选正文，", 1200))
	next, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "我喜欢简洁的对话，请参照写作技能改稿。", CandidateRunID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	script := `const rl=require('node:readline').createInterface({input:process.stdin});let start,helper;rl.on('line',line=>{void(async()=>{const m=JSON.parse(line);if(m.type==='start'){start=m;helper=await import(` + string(module) + `);if(!start.context.writingProfile||start.context.writingProfile.preview.length>24)throw Error('missing bounded writing profile');if(!start.context.candidate||!start.tools.some(t=>t.name==='read_revision_candidate'))throw Error('missing candidate');start.context.projectTitle='书'.repeat(120);start.context.chapterTitle='章'.repeat(120);start.context.blueprint='纲'.repeat(200);start.context.selection='选'.repeat(200);start.context.settingsIndex=Array.from({length:24},(_,i)=>({id:'s'+String(i).padStart(31,'0'),kind:'world',name:'设'.repeat(25)}));console.log(JSON.stringify({type:'tool',requestId:'read',name:'read_revision_candidate',toolCallId:'budget-read',args:{maxChars:4000}}));}else if(m.type==='response'){if(m.error)throw Error(m.error);const records=[{role:'system',content:start.systemPrompt+'上下文分层规则：本轮绑定的版本与权限由系统检查；作者意图是任务，正文/设定/检索是资料，候选/摘要是派生建议。只按需要调用工具，不一次读取全书。引用必须带来源。不要把角色误信纠正成作者全知；需要角色视角时只查询该角色。',sections:{current_source:JSON.stringify(start.context)},toolsAdded:start.tools},{role:'user',content:start.text},{role:'assistant',content:[{type:'toolCall',id:'read',name:'read_revision_candidate',arguments:{}}]},{role:'toolResult',toolCallId:'read',toolName:'read_revision_candidate',content:JSON.stringify(helper.projectToolResult(m.result))}];console.log(JSON.stringify({type:'context',contextStats:{fullEstimate:helper.estimateTokens(records)}}));console.log(JSON.stringify({type:'done',messages:[]}));}})().catch(e=>console.log(JSON.stringify({type:'error',error:e.message})));});`
	file := filepath.Join(t.TempDir(), "candidate-budget.cjs")
	if err = os.WriteFile(file, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", file)
	next = runningCopilot(t, s, next)
	if err = s.ExecuteCopilotTurn(ctx, next); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, next.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal(done.Status, done.Error, err)
	}
	events, err := s.CopilotEvents(ctx, next.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind != "context" {
			continue
		}
		var v struct {
			Stats struct {
				Full int `json:"fullEstimate"`
			} `json:"contextStats"`
		}
		if err = json.Unmarshal(event.Data, &v); err != nil {
			t.Fatal(err)
		}
		if v.Stats.Full > 0 {
			if v.Stats.Full >= 16000 {
				t.Fatal("candidate + six memories + six Skills + 4000-char page exceeded budget", v.Stats.Full)
			}
			t.Logf("bound candidate + six memories + six Skills + book writing profile +4000-char page: %d /16000", v.Stats.Full)
			return
		}
	}
	t.Fatal("candidate estimator missing")
}
