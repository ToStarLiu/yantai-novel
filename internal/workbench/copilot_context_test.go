package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel/internal/job"
	"novel/internal/llm"
)

func TestCopilotPagedSelectionAndBlueprintPreserveFullReplacementAnchor(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	selection := strings.Repeat("选段", 600) + "末尾锚点"
	r, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("前文。" + selection + "后文。")})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.UpdateChapter(ctx, c.ID, c.Title, c.Volume, strings.Repeat("章纲", 5000)+"最终目标")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := s.NewCopilotToolContext(ctx, job.NewID(), c.ProjectID, c.ID, r.ID, selection)
	if err != nil {
		t.Fatal(err)
	}
	read, err := s.ExecuteCopilotTool(ctx, scope, "read_chapter", []byte(`{"maxChars":200}`))
	if err != nil {
		t.Fatal(err)
	}
	content := read.Content.(map[string]any)
	if content["selectionTruncated"] != true || content["blueprintTruncated"] != true || content["selectionReadTool"] != "read_selection" || content["blueprintReadTool"] != "read_blueprint" {
		t.Fatal("missing continuation markers", content)
	}
	var reconstructed strings.Builder
	for offset := 0; offset < len([]rune(selection)); {
		page, err := s.ExecuteCopilotTool(ctx, scope, "read_selection", copilotArgs(map[string]any{"offset": offset, "maxChars": 600}))
		if err != nil {
			t.Fatal(err)
		}
		value := page.Content.(map[string]any)
		if value["selectionHash"] != job.Hash([]byte(selection)) || value["sourceCharCount"] != len([]rune(selection)) {
			t.Fatal("selection identity missing", value)
		}
		reconstructed.WriteString(value["text"].(string))
		offset = value["nextOffset"].(int)
	}
	if reconstructed.String() != selection || scope.Selection != selection {
		t.Fatal("selection continuation or original anchor changed")
	}
	changed, err := s.NewCopilotToolContext(ctx, job.NewID(), c.ProjectID, c.ID, r.ID, "末尾锚点")
	if err != nil {
		t.Fatal(err)
	}
	changedPage, err := s.ExecuteCopilotTool(ctx, changed, "read_selection", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if changedPage.Content.(map[string]any)["selectionHash"] == job.Hash([]byte(selection)) || changedPage.Content.(map[string]any)["sourceCharCount"] != 4 {
		t.Fatal("selection identity did not follow current selection", changedPage)
	}
	page, err := s.ExecuteCopilotTool(ctx, scope, "read_blueprint", []byte(`{"offset":10000,"maxChars":600}`))
	if err != nil {
		t.Fatal(err)
	}
	if page.Content.(map[string]any)["text"] != "最终目标" {
		t.Fatal("blueprint tail inaccessible", page)
	}
	viewpoint := scope
	viewpoint.PointOfViewCharacterID = "character-locked"
	if _, err = s.ExecuteCopilotTool(ctx, viewpoint, "read_blueprint", []byte(`{}`)); !errors.Is(err, ErrForbidden) {
		t.Fatal("narrator outline leaked to character viewpoint", err)
	}
	candidate, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"text":"替换全文。","mode":"selection"}`))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Artifact.Text != "前文。替换全文。后文。" {
		t.Fatal("partial projection replaced only part of the selection", candidate.Artifact.Text)
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil || current.CurrentRevisionID != r.ID {
		t.Fatal("candidate changed source", err)
	}
}

func TestCopilotRealPiLongBlueprintAndSettingNamesFitInitialContext(t *testing.T) {
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
	outline := strings.Repeat("这是很长的作者章纲。", 3000)
	var err error
	r, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(strings.Repeat("这是一章很长的中文正文。", 2000))})
	if err != nil {
		t.Fatal(err)
	}
	c.CurrentRevisionID = r.ID
	c, err = s.UpdateChapter(ctx, c.ID, c.Title, c.Volume, outline)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if _, err = s.SaveSetting(ctx, c.ProjectID, Setting{Kind: "world", Name: fmt.Sprintf("%02d", i) + strings.Repeat("长设定名称", 45)}); err != nil {
			t.Fatal(err)
		}
	}
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	if err = s.ExecuteCopilotTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, turn.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal("long immutable source prevented normal tool usage", done.Status, done.Error, err)
	}
	events, err := s.CopilotEvents(ctx, turn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	read := false
	for _, event := range events {
		if event.Kind == "tool-result" && strings.Contains(string(event.Data), "read_chapter") {
			read = true
			break
		}
	}
	if !read {
		t.Fatal("source read not in durable archive")
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil || current.Blueprint != outline {
		t.Fatal("model projection changed author outline", err)
	}
}

func TestCopilotSourcePagesFitWindowAndKeepEveryCharacter(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	body := strings.Repeat("甲", 9000) + "正文结尾"
	selection := strings.Repeat("甲", 2500)
	// A unique selection must retain its exact boundary while projection is short.
	body = "选段开始" + selection + "选段结束" + body
	selection = "选段开始" + selection + "选段结束"
	r, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(body)})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.UpdateChapter(ctx, c.ID, c.Title, c.Volume, strings.Repeat("纲", 10000))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := s.NewCopilotToolContext(ctx, job.NewID(), c.ProjectID, c.ID, r.ID, selection)
	if err != nil {
		t.Fatal(err)
	}
	estimate := func(content any) int {
		raw, _ := json.Marshal(content)
		ascii, other := 0, 0
		for _, r := range string(raw) {
			if r < 128 {
				ascii++
			} else {
				other++
			}
		}
		return int(math.Ceil(float64(ascii)/3+float64(other)*1.7)) + 16
	}
	var restored strings.Builder
	for offset := 0; offset < len([]rune(body)); {
		page, err := s.ExecuteCopilotTool(ctx, scope, "read_chapter", copilotArgs(map[string]any{"offset": offset}))
		if err != nil {
			t.Fatal(err)
		}
		value := page.Content.(map[string]any)
		if n := estimate(page.Content); n >= 10000 {
			t.Fatal("one source page leaves insufficient room for current instructions", n)
		}
		text := value["text"].(string)
		if len([]rune(text)) > 4000 || len([]rune(value["blueprint"].(string))) > 500 || len([]rune(value["selection"].(string))) > 500 {
			t.Fatal("source projection exceeds per-field cap")
		}
		if value["nextOffset"].(int) != offset+len([]rune(text)) {
			t.Fatal("source cursor skipped unreturned characters")
		}
		restored.WriteString(text)
		offset = value["nextOffset"].(int)
	}
	if restored.String() != body {
		t.Fatal("chapter pagination skipped or duplicated text")
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_chapter", []byte(`{"maxChars":4001}`)); err == nil {
		t.Fatal("unsafe source page accepted")
	}
	wanted := map[string]string{}
	for i := 0; i < 7; i++ {
		text := strings.Repeat(string(rune('乙'+i)), 5000) + fmt.Sprint("尾", i)
		item, err := s.SaveSetting(ctx, c.ProjectID, Setting{Kind: "world", Name: fmt.Sprintf("分页设定%02d", i), Content: text})
		if err != nil {
			t.Fatal(err)
		}
		wanted[item.ID] = text
	}
	// Refresh after author settings update to use the new epoch.
	scope, err = s.NewCopilotToolContext(ctx, job.NewID(), c.ProjectID, c.ID, r.ID, selection)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for offset := 0; len(seen) < len(wanted); {
		page, err := s.ExecuteCopilotTool(ctx, scope, "read_settings", copilotArgs(map[string]any{"query": "分页设定", "offset": offset}))
		if err != nil {
			t.Fatal(err)
		}
		value := page.Content.(map[string]any)
		if n := estimate(value); n >= 10000 {
			t.Fatal("one settings result exceeds safe source budget", n)
		}
		items := value["settings"].([]map[string]any)
		if len(items) == 0 {
			t.Fatal("settings pagination ended early", offset)
		}
		chars := 0
		for _, item := range items {
			id := item["id"].(string)
			if seen[id] {
				t.Fatal("list cursor duplicated a setting", id)
			}
			seen[id] = true
			text := item["content"].(string)
			chars += len([]rune(text))
			if len([]rune(text)) > 2000 {
				t.Fatal("one setting exceeds cap")
			}
			var full strings.Builder
			full.WriteString(text)
			for cursor := item["nextContentOffset"].(int); cursor < item["totalChars"].(int); {
				next, err := s.ExecuteCopilotTool(ctx, scope, "read_settings", copilotArgs(map[string]any{"id": id, "contentOffset": cursor}))
				if err != nil {
					t.Fatal(err)
				}
				detail := next.Content.(map[string]any)["settings"].([]map[string]any)[0]
				piece := detail["content"].(string)
				if detail["nextContentOffset"].(int) != cursor+len([]rune(piece)) {
					t.Fatal("content cursor skipped text")
				}
				full.WriteString(piece)
				cursor = detail["nextContentOffset"].(int)
			}
			if full.String() != wanted[id] {
				t.Fatal("setting continuation lost content", id)
			}
		}
		if chars > 4000 {
			t.Fatal("settings aggregate exceeded cap", chars)
		}
		offset = value["nextOffset"].(int)
	}
	viewpoint := scope
	viewpoint.PointOfViewCharacterID = "character-locked"
	for _, query := range []string{"分页设定", "乙乙乙乙"} {
		page, err := s.ExecuteCopilotTool(ctx, viewpoint, "read_settings", copilotArgs(map[string]any{"query": query}))
		if err != nil {
			t.Fatal(err)
		}
		items := page.Content.(map[string]any)["settings"].([]map[string]any)
		if query == "乙乙乙乙" && len(items) != 0 {
			t.Fatal("POV query matched hidden content")
		}
		for _, item := range items {
			if item["content"] != "" || item["totalChars"] != 0 || item["nextContentOffset"] != 0 {
				t.Fatal("POV leaked content or hidden length", item)
			}
		}
	}
	for id := range wanted {
		if _, err = s.ExecuteCopilotTool(ctx, viewpoint, "read_settings", copilotArgs(map[string]any{"id": id, "contentOffset": 1})); !errors.Is(err, ErrForbidden) {
			t.Fatal("POV continued hidden content", err)
		}
		break
	}
}

func TestCopilotHostStopsAfterUnfinishedOrUnknownAttempt(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := context.Background()
	turn := runningCopilot(t, s, submitCopilot(t, s, c, session))
	request := copilotWire{InputTokensEstimate: 10, MaxOutputTokens: 256, Messages: json.RawMessage(`[]`)}
	id, err := s.copilotAuthorize(ctx, turn, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.copilotAuthorize(ctx, turn, request); !IsConflict(err) {
		t.Fatal("overlapping calls authorized", err)
	}
	if err = s.copilotUsage(ctx, turn, copilotWire{AttemptID: id, Status: "NeedsReconciliation", Usage: llm.Usage{Reported: false}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.copilotAuthorize(ctx, turn, request); !IsConflict(err) {
		t.Fatal("unknown result automatically retried", err)
	}
	if err = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, turn.ID)
	if err != nil || done.Status != "NeedsReconciliation" {
		t.Fatal("harness completion hid unknown attempt", done, err)
	}
	view, err := s.GetCopilotSession(ctx, session.ID)
	if err != nil || view.Session.Version != 0 {
		t.Fatal("unknown result overwrote canonical history", err)
	}
	attempts, err := s.CopilotAttempts(ctx, turn.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatal("unexpected paid replay", attempts, err)
	}
}

// Exercise actual Go startup metadata + tool schemas + tool response through the
// harness estimator. The fixture dispatches no provider call and stores only sizes.
func estimateCopilotRead(t *testing.T, s *Service, turn CopilotTurn, tool string) int {
	t.Helper()
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	contextFile, _ := filepath.Abs("../../harness/dist/context.js")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err := os.Stat(contextFile); err != nil {
		t.Skip("Build Pi context module")
	}
	module, _ := json.Marshal("file:///" + filepath.ToSlash(contextFile))
	toolName, _ := json.Marshal(tool)
	script := `const rl=require('node:readline').createInterface({input:process.stdin});let start;let helper;rl.on('line',line=>{void(async()=>{const m=JSON.parse(line);if(m.type==='start'){start=m;helper=await import(` + string(module) + `);console.log(JSON.stringify({type:'tool',requestId:'read',name:` + string(toolName) + `,toolCallId:'estimate-read',args:` + string(toolName) + `==='search_story_memory'?{query:'事实',limit:8,...(start.context.scope!=='book'?{characterId:start.context.settingsIndex.find(s=>s.kind==='character')?.id}:{})}:{}}  ));}else if(m.type==='response'){if(m.error){console.log(JSON.stringify({type:'error',error:m.error}));return;}const projected=helper.projectToolResult(m.result);start.context.projectTitle='书'.repeat(120);start.context.chapterTitle='章'.repeat(120);start.context.settingsIndex=Array.from({length:24},(_,i)=>({id:'s'+String(i).padStart(31,'0'),kind:'world',name:'设'.repeat(25)}));if(start.context.scope!=='book'){start.context.blueprint='纲'.repeat(500);start.context.selection='选'.repeat(1000);}const records=[{role:'system',content:start.systemPrompt+'上下文分层规则：本轮绑定的版本与权限由系统检查；作者意图是任务，正文/设定/检索是资料，候选/摘要是派生建议。只按需要调用工具，不一次读取全书。引用必须带来源。不要把角色误信纠正成作者全知；需要角色视角时只查询该角色。',sections:{current_source:JSON.stringify(start.context)},toolsAdded:start.tools},{role:'user',content:start.text},{role:'assistant',content:[{type:'toolCall',id:'fixture-call',name:` + string(toolName) + `,arguments:{}}]},{role:'toolResult',toolCallId:'fixture-call',toolName:` + string(toolName) + `,content:JSON.stringify(projected)}];const full=helper.estimateTokens(records);console.log(JSON.stringify({type:'context',contextStats:{fullEstimate:full,toolEstimate:helper.estimateTokens(projected)}}));console.log(JSON.stringify({type:'done',messages:[]}));}})().catch(()=>console.log(JSON.stringify({type:'error',error:'estimate fixture failed'})));});`
	entry := filepath.Join(t.TempDir(), "estimate.cjs")
	if err := os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	if err := s.ExecuteCopilotTurn(copilotAdmin(), turn); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(copilotAdmin(), turn.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal("source estimate fixture failed", done.Status, done.Error, err)
	}
	events, err := s.CopilotEvents(copilotAdmin(), turn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind != "context" {
			continue
		}
		var result struct {
			Stats struct {
				Full int `json:"fullEstimate"`
				Tool int `json:"toolEstimate"`
			} `json:"contextStats"`
		}
		if err := json.Unmarshal(event.Data, &result); err != nil {
			t.Fatal(err)
		}
		if result.Stats.Full > 0 {
			if result.Stats.Full >= 14000 || result.Stats.Tool >= 9000 {
				t.Fatalf("%s exceeded safe input margin: full=%d tool=%d", tool, result.Stats.Full, result.Stats.Tool)
			}
			t.Logf("%s actual harness estimate including startup + tool schemas: full=%d tool=%d, input budget=16000", tool, result.Stats.Full, result.Stats.Tool)
			return result.Stats.Full
		}
	}
	t.Fatal("harness estimate not persisted")
	return 0
}

func TestCopilotBookAndCheckSingleResultFitActualHarnessBudget(t *testing.T) {
	t.Run("book-overview", func(t *testing.T) {
		s, p, session := bookCopilotFixture(t)
		ctx := copilotAdmin()
		brief := StoryBrief{PlotPlan: strings.Repeat("线", 5000), Genre: strings.Repeat("题", 500), Audience: strings.Repeat("受", 500), POV: strings.Repeat("视", 500), Platform: strings.Repeat("台", 500), Theme: strings.Repeat("主", 500), SellingPoint: strings.Repeat("卖", 500), CoreConflict: strings.Repeat("冲", 500), ProtagonistDesire: strings.Repeat("欲", 500), FailureCost: strings.Repeat("价", 500), EndingDirection: strings.Repeat("终", 500), Restrictions: strings.Repeat("禁", 500), Style: strings.Repeat("风", 500), Pace: strings.Repeat("节", 500)}
		if _, err := s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{ExpectedVersion: 0, Data: brief}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 25; i++ {
			if _, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "world", Name: fmt.Sprint(i) + strings.Repeat("设", 80), Content: strings.Repeat("界", 2000)}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateChapter(ctx, p.ID, fmt.Sprint(i)+strings.Repeat("章", 80), strings.Repeat("卷", 80)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.UpdateProject(ctx, p.ID, p.Title, strings.Repeat("简", 2000), strings.Repeat("题", 100)); err != nil {
			t.Fatal(err)
		}
		turn := bookTurn(t, s, session)
		seen := map[string]bool{}
		for offset := 0; len(seen) < 25; {
			result, err := s.ExecuteBookCopilotTool(ctx, turn, "read_book_brief", "page", copilotArgs(map[string]any{"chapterOffset": offset, "chapterLimit": 20}))
			if err != nil {
				t.Fatal(err)
			}
			value := result.Content.(map[string]any)
			chapters := value["chapters"].([]map[string]any)
			if len(chapters) == 0 || len(chapters) > 20 {
				t.Fatal("directory pagination failed", offset, len(chapters))
			}
			for _, chapter := range chapters {
				id := chapter["id"].(string)
				if seen[id] {
					t.Fatal("directory repeated source", id)
				}
				seen[id] = true
				if len([]rune(chapter["title"].(string))) >= 60 || len([]rune(chapter["volume"].(string))) >= 60 {
					t.Fatal("directory title not bounded")
				}
			}
			next := value["nextChapterOffset"].(int)
			if next != offset+len(chapters) {
				t.Fatal("directory cursor skipped source")
			}
			offset = next
		}
		if _, err := s.ExecuteBookCopilotTool(ctx, turn, "read_book_brief", "bad", []byte(`{"chapterLimit":21}`)); err == nil {
			t.Fatal("unbounded directory page accepted")
		}
		estimateCopilotRead(t, s, turn, "read_book_brief")
	})
	t.Run("local-check", func(t *testing.T) {
		f := newNarrativeFixture(t)
		ctx := copilotAdmin()
		for i := 0; i < 24; i++ {
			if _, err := f.s.SaveSetting(ctx, f.p.ID, Setting{Kind: "world", Name: fmt.Sprint(i) + strings.Repeat("名", 80), Content: strings.Repeat("设", 3000)}); err != nil {
				t.Fatal(err)
			}
		}
		quotes := []string{}
		for i := 0; i < 20; i++ {
			quotes = append(quotes, fmt.Sprint("证据", i)+strings.Repeat("往", 650))
		}
		past := f.chapter(t, "前章", strings.Join(quotes, "\n"))
		changes := []MemoryChange{}
		for i, quote := range quotes {
			object := strings.Repeat("事实", 350) + fmt.Sprint(i)
			if i == 0 {
				object = strings.Repeat("完整字段", 800)
			}
			changes = append(changes, f.change(t, past, "Knowledge", f.seal.ID, fmt.Sprintf("属性%02d", i), object, f.lin.ID, "Unknown", quote))
		}
		f.apply(t, f.proposal(t, past, changes))
		c, err := f.s.CreateChapter(ctx, f.p.ID, "检查本章", "")
		if err != nil {
			t.Fatal(err)
		}
		paragraphs := []string{}
		for i := 0; i < 20; i++ {
			paragraphs = append(paragraphs, fmt.Sprint("当前", i)+strings.Repeat("段", 700))
		}
		current := strings.Join(paragraphs, "\n")
		r, err := f.s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(current)})
		if err != nil {
			t.Fatal(err)
		}
		scope, err := f.s.NewCopilotToolContext(ctx, job.NewID(), f.p.ID, c.ID, r.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		firstID := ""
		for offset := 0; len(seen) < 20; {
			result, err := f.s.ExecuteCopilotTool(ctx, scope, "check_consistency", copilotArgs(map[string]any{"offset": offset}))
			if err != nil {
				t.Fatal(err)
			}
			value := result.Content.(map[string]any)
			facts := value["confirmedFacts"].([]map[string]any)
			if len(facts) == 0 || len(facts) > 8 {
				t.Fatal("fact pagination failed", offset)
			}
			budget := 0
			for _, fact := range facts {
				id := fact["id"].(string)
				if seen[id] {
					t.Fatal("fact cursor repeated source")
				}
				seen[id] = true
				if strings.HasSuffix(fact["predicate"].(string), "00") {
					firstID = id
				}
				object := fact["object"].(string)
				quote := fact["source"].(map[string]any)["quote"].(string)
				if len([]rune(object)) > 200 || len([]rune(quote)) > 120 {
					t.Fatal("fact preview too large")
				}
				budget += len([]rune(object)) + len([]rune(quote)) + len([]rune(fact["predicate"].(string)))
			}
			if budget > 2000 {
				t.Fatal("facts aggregate too large", budget)
			}
			next := value["nextOffset"].(int)
			if next != offset+len(facts) {
				t.Fatal("fact cursor skipped source")
			}
			offset = next
		}
		var full strings.Builder
		for offset := 0; offset < len([]rune(changes[0].Object)); {
			result, err := f.s.ExecuteCopilotTool(ctx, scope, "check_consistency", copilotArgs(map[string]any{"factId": firstID, "field": "object", "fieldOffset": offset}))
			if err != nil {
				t.Fatal(err)
			}
			value := result.Content.(map[string]any)
			text := value["text"].(string)
			full.WriteString(text)
			next := value["nextOffset"].(int)
			if next != offset+len([]rune(text)) {
				t.Fatal("fact detail skipped text")
			}
			offset = next
		}
		if full.String() != changes[0].Object {
			t.Fatal("fact field continuation incomplete")
		}
		seenIssues := map[int]bool{}
		issueOffset := 0
		for {
			result, err := f.s.ExecuteCopilotTool(ctx, scope, "check_consistency", copilotArgs(map[string]any{"issueOffset": issueOffset}))
			if err != nil {
				t.Fatal(err)
			}
			value := result.Content.(map[string]any)
			issues := value["issues"].([]map[string]any)
			if len(issues) > 4 {
				t.Fatal("too many local issues")
			}
			for _, issue := range issues {
				index := issue["index"].(int)
				if seenIssues[index] {
					t.Fatal("issue cursor repeated")
				}
				seenIssues[index] = true
				at := issue["quoteOffset"].(int)
				quote := issue["quote"].(string)
				if at < 0 || !strings.HasPrefix(string([]rune(current)[at:]), quote) {
					t.Fatal("issue quote continuation missing")
				}
			}
			if value["issuesHasMore"] != true {
				break
			}
			next := value["nextIssueOffset"].(int)
			if next <= issueOffset {
				t.Fatal("issue cursor stalled")
			}
			issueOffset = next
		}
		if len(seenIssues) < 20 {
			t.Fatal("local checks lost issues", len(seenIssues))
		}
		viewpoint := scope
		viewpoint.PointOfViewCharacterID = f.lin.ID
		if _, err = f.s.ExecuteCopilotTool(ctx, viewpoint, "check_consistency", copilotArgs(map[string]any{"factId": firstID, "field": "quote"})); !errors.Is(err, ErrForbidden) {
			t.Fatal("fact evidence leaked to POV", err)
		}
		f.s.ProviderName = "fake"
		f.s.Model = "fake-v1"
		f.s.MaxOutputTokens = 500
		session, err := f.s.CreateCopilotSession(ctx, f.p.ID, CopilotSessionInput{ChapterID: c.ID})
		if err != nil {
			t.Fatal(err)
		}
		turn, err := f.s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "检查当前章与确认来源"})
		if err != nil {
			t.Fatal(err)
		}
		estimateCopilotRead(t, f.s, runningCopilot(t, f.s, turn), "check_consistency")
		for _, tool := range []string{"read_chapter", "read_settings", "search_story_memory"} {
			next, err := f.s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "读取当前有效来源"})
			if err != nil {
				t.Fatal(err)
			}
			estimateCopilotRead(t, f.s, runningCopilot(t, f.s, next), tool)
		}
		bookSession, err := f.s.CreateCopilotSession(ctx, f.p.ID, CopilotSessionInput{Scope: "book"})
		if err != nil {
			t.Fatal(err)
		}
		estimateCopilotRead(t, f.s, bookTurn(t, f.s, bookSession), "search_story_memory")
	})
}
