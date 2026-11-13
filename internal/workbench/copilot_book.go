package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

func boundedBookBrief(in StoryBrief) (StoryBrief, bool) {
	out := in
	budget := 1800
	truncated := false
	trim := func(text string, max int) string {
		if max > budget {
			max = budget
		}
		v, cut := boundedCopilotText(text, max)
		budget -= len([]rune(v))
		truncated = truncated || cut
		return v
	}
	out.PlotPlan = trim(in.PlotPlan, 600)
	for _, pair := range []struct {
		to   *string
		from string
	}{{&out.Genre, in.Genre}, {&out.Audience, in.Audience}, {&out.POV, in.POV}, {&out.Platform, in.Platform}, {&out.Theme, in.Theme}, {&out.SellingPoint, in.SellingPoint}, {&out.CoreConflict, in.CoreConflict}, {&out.ProtagonistDesire, in.ProtagonistDesire}, {&out.FailureCost, in.FailureCost}, {&out.EndingDirection, in.EndingDirection}, {&out.Relationships, in.Relationships}, {&out.Restrictions, in.Restrictions}, {&out.Style, in.Style}, {&out.Pace, in.Pace}} {
		*pair.to = trim(pair.from, 100)
	}
	out.WorldRules = []string{}
	for _, rule := range in.WorldRules {
		if len(out.WorldRules) == 6 || budget <= 0 {
			truncated = true
			break
		}
		out.WorldRules = append(out.WorldRules, trim(rule, 100))
	}
	out.Volumes = []StoryVolume{}
	for _, volume := range in.Volumes {
		if len(out.Volumes) == 6 || budget <= 0 {
			truncated = true
			break
		}
		v := volume
		v.Title = trim(v.Title, 40)
		v.Goal = trim(v.Goal, 100)
		v.Summary = trim(v.Summary, 100)
		out.Volumes = append(out.Volumes, v)
	}
	return out, truncated
}

// The book baseline has no chapter dependency. A project with zero chapters is
// a first-class planning context, while authoring sessions keep chapter fences.
func copilotSnapshotCurrent(ctx context.Context, db snapshotDB, t CopilotTurn) (bool, error) {
	if t.Input.AuthorMemory != nil && t.Input.AuthorMemory.OwnerID != t.Input.Principal.ID {
		return false, nil
	}
	if t.Input.Scope != "book" {
		return snapshotCurrent(ctx, db, copilotSnapshot(t))
	}
	var epoch int64
	var state string
	if err := db.QueryRow(ctx, "SELECT epoch,state_version_id FROM wb_projects WHERE id=$1 FOR SHARE", t.ProjectID).Scan(&epoch, &state); err != nil {
		return false, noRows(err)
	}
	if epoch != t.Input.Epoch || state != t.Input.StateVersionID || len(t.Input.StudioVersions) != 3 {
		return false, nil
	}
	if valid, err := copilotAuthorMemoryCurrent(ctx, db, t.Input); err != nil || !valid {
		return valid, err
	}
	if valid, err := writingSkillSnapshotCurrent(ctx, db, t.ProjectID, t.Input.WritingSkills); err != nil || !valid {
		return valid, err
	}
	index, err := bookIndexHash(ctx, db, t.ProjectID)
	if err != nil {
		return false, err
	}
	if index != t.Input.BookIndexHash {
		return false, nil
	}
	if tx, ok := db.(pgx.Tx); ok {
		return ValidateStudioPromptTx(ctx, tx, t.ProjectID, t.Input.StudioVersions)
	}
	for _, kind := range []string{"brief", "scenes", "arcs"} {
		var version int64
		if err := db.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM wb_studio_documents WHERE project_id=$1 AND kind=$2 AND id='main'),0)", t.ProjectID, kind).Scan(&version); err != nil {
			return false, err
		}
		expected, ok := t.Input.StudioVersions[kind]
		if !ok || version != expected {
			return false, nil
		}
	}
	return true, nil
}

func bookIndexHash(ctx context.Context, db snapshotDB, pid string) (string, error) {
	var encoded []byte
	err := db.QueryRow(ctx, `SELECT jsonb_build_object('title',p.title,'description',p.description,'genre',p.genre,'chapters',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',c.id,'title',c.title,'volume',c.volume,'order',c.sort_order,'revision',c.current_revision_id,'final',c.final_revision_id) ORDER BY c.sort_order,c.id) FROM wb_chapters c WHERE c.project_id=p.id AND NOT c.deleted),'[]'::jsonb)) FROM wb_projects p WHERE p.id=$1`, pid).Scan(&encoded)
	if err != nil {
		return "", noRows(err)
	}
	return job.Hash(encoded), nil
}

func (s *Service) CopilotToolsForScope(scope string) []CopilotToolDefinition {
	if scope != "book" {
		return s.CopilotTools()
	}
	return []CopilotToolDefinition{
		{"read_book_brief", "读取当前全书规划、作者设定摘要和卷章目录。即使尚无章节也可使用；不读取整本正文、不把未来计划当事实。概览文字合计约4000字；目录默认10项、最多20项，chapterOffset/chapterLimit续读。长规划可用plotOffset续读。其他字段截断时传field字段名、fieldOffset续读完整字段；worldRules/volumes以JSON文本分页。", json.RawMessage(`{"type":"object","properties":{"plotOffset":{"type":"integer","minimum":0},"chapterOffset":{"type":"integer","minimum":0},"chapterLimit":{"type":"integer","minimum":1,"maximum":20},"field":{"type":"string","enum":["plotPlan","genre","audience","pov","platform","theme","sellingPoint","coreConflict","protagonistDesire","failureCost","endingDirection","worldRules","relationships","restrictions","style","pace","volumes"]},"fieldOffset":{"type":"integer","minimum":0}},"additionalProperties":false}`)},
		{"read_settings", "按类型、名称或内容检索作者设定；用offset翻页或用id和contentOffset续读单条长设定。全书模式属于作者规划视角，不表示任何角色已知这些信息。", json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"kind":{"type":"string","enum":["","character","world","rule","style","outline"]},"query":{"type":"string","maxLength":500},"offset":{"type":"integer","minimum":0},"contentOffset":{"type":"integer","minimum":0}},"additionalProperties":false}`)},
		{"search_story_memory", "按关键词检索全书当前有效的已确认事实，不读整本正文。返回原始来源和Knowledge获知者/误信状态，供作者规划参考，不能视为任何角色的共同知识。", json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":500},"limit":{"type":"integer","minimum":1,"maximum":8},"offset":{"type":"integer","minimum":0},"factId":{"type":"string"},"field":{"type":"string","enum":["object","quote"]},"fieldOffset":{"type":"integer","minimum":0}},"required":[],"additionalProperties":false}`)},
		{"propose_book_plan", "保存你与作者讨论出的全书计划为独立候选，供作者明确采用到全书规划。可包含题材、核心冲突、角色弧、世界规则、各卷目标、伏笔与结局方向；不自动创建章节、修改设定或确认事实。", json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","minLength":1,"maxLength":40000},"title":{"type":"string","maxLength":120}},"required":["text"],"additionalProperties":false}`)},
	}
}

func (s *Service) bookToolTx(ctx context.Context, t CopilotTurn, minimum string) (pgx.Tx, error) {
	if t.Input.Scope != "book" || t.OwnerID != copilotOwner(ctx) {
		return nil, ErrForbidden
	}
	if err := s.RequireProjectRole(ctx, t.ProjectID, minimum); err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR SHARE", t.ProjectID).Scan(&id); err == nil {
		err = s.checkCopilotCurrentTx(ctx, tx, t)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func (s *Service) ExecuteBookCopilotTool(ctx context.Context, t CopilotTurn, name, callID string, args json.RawMessage) (CopilotToolResult, error) {
	if name == "read_author_memory" || name == "propose_author_memory" {
		return s.executeCopilotAuthorMemoryTool(ctx, t, name, args)
	}
	minimum := "viewer"
	if name == "propose_book_plan" {
		minimum = "editor"
	}
	tx, err := s.bookToolTx(ctx, t, minimum)
	if err != nil {
		return CopilotToolResult{}, err
	}
	defer tx.Rollback(ctx)
	switch name {
	case "read_writing_skill":
		out, err := readFrozenWritingSkill(t.Input.WritingSkills, args)
		if err != nil {
			return out, err
		}
		return out, tx.Commit(ctx)
	case "read_book_brief":
		var page struct {
			PlotOffset    int    `json:"plotOffset"`
			ChapterOffset int    `json:"chapterOffset"`
			ChapterLimit  int    `json:"chapterLimit"`
			Field         string `json:"field"`
			FieldOffset   int    `json:"fieldOffset"`
		}
		if err = copilotDecode(args, &page); err != nil {
			return CopilotToolResult{}, err
		}
		if page.ChapterLimit == 0 {
			page.ChapterLimit = 10
		}
		if page.PlotOffset < 0 || page.ChapterOffset < 0 || page.ChapterOffset > 1000000 || page.ChapterLimit < 1 || page.ChapterLimit > 20 || page.FieldOffset < 0 || (page.Field == "" && page.FieldOffset != 0) {
			return CopilotToolResult{}, fmt.Errorf("规划续读位置无效")
		}
		brief, err := studioRead(ctx, tx, t.ProjectID, "brief", "main", StoryBrief{WorldRules: []string{}, Volumes: []StoryVolume{}})
		if err != nil {
			return CopilotToolResult{}, err
		}
		if page.Field != "" {
			allowed := false
			for _, field := range []string{"plotPlan", "genre", "audience", "pov", "platform", "theme", "sellingPoint", "coreConflict", "protagonistDesire", "failureCost", "endingDirection", "worldRules", "relationships", "restrictions", "style", "pace", "volumes"} {
				if page.Field == field {
					allowed = true
					break
				}
			}
			if !allowed {
				return CopilotToolResult{}, fmt.Errorf("规划字段无效")
			}
			encoded, _ := json.Marshal(brief.Data)
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(encoded, &fields); err != nil {
				return CopilotToolResult{}, err
			}
			fieldText := string(fields[page.Field])
			var text string
			if json.Unmarshal(fields[page.Field], &text) == nil {
				fieldText = text
			}
			runes := []rune(fieldText)
			if page.FieldOffset > len(runes) {
				return CopilotToolResult{}, fmt.Errorf("规划字段续读超出范围")
			}
			end := min(len(runes), page.FieldOffset+2000)
			out := CopilotToolResult{Content: map[string]any{"source": "author-book-brief-field", "projectId": t.ProjectID, "briefVersion": brief.Version, "field": page.Field, "text": string(runes[page.FieldOffset:end]), "offset": page.FieldOffset, "nextOffset": end, "totalChars": len(runes), "truncated": end < len(runes) || page.FieldOffset > 0, "authority": "这是作者规划字段的完整来源分页；规则属于创作约束，未来计划不是已发生事实。"}}
			return out, tx.Commit(ctx)
		}
		var project Project
		if project, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1", t.ProjectID)); err != nil {
			return CopilotToolResult{}, err
		}
		settings := []Setting{}
		truncated := false
		budget := 600
		rows, err := tx.Query(ctx, "SELECT id,project_id,kind,name,content FROM wb_settings WHERE project_id=$1 ORDER BY kind,name,id LIMIT 31", t.ProjectID)
		if err != nil {
			return CopilotToolResult{}, err
		}
		for rows.Next() {
			var item Setting
			if err = rows.Scan(&item.ID, &item.ProjectID, &item.Kind, &item.Name, &item.Content); err != nil {
				rows.Close()
				return CopilotToolResult{}, err
			}
			if len(settings) >= 6 || budget <= 40 {
				truncated = true
				continue
			}
			name, nameCut := boundedCopilotText(item.Name, min(40, budget))
			budget -= len([]rune(name))
			max := 180
			if budget < max {
				max = budget
			}
			var cut bool
			item.Content, cut = boundedCopilotText(item.Content, max)
			item.Name = name
			truncated = truncated || cut || nameCut
			budget -= len([]rune(item.Content))
			settings = append(settings, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return CopilotToolResult{}, err
		}
		chapters := []map[string]any{}
		chapterBudget := 1000
		chapterHasMore := false
		rows, err = tx.Query(ctx, "SELECT id,title,volume,sort_order,current_revision_id,final_revision_id FROM wb_chapters WHERE project_id=$1 AND NOT deleted ORDER BY sort_order,id LIMIT $3 OFFSET $2", t.ProjectID, page.ChapterOffset, page.ChapterLimit+1)
		if err != nil {
			return CopilotToolResult{}, err
		}
		for rows.Next() {
			var id, title, volume, revision, final string
			var order int
			if err = rows.Scan(&id, &title, &volume, &order, &revision, &final); err != nil {
				rows.Close()
				return CopilotToolResult{}, err
			}
			if len(chapters) == page.ChapterLimit || chapterBudget < 70 {
				truncated = true
				chapterHasMore = true
				continue
			}
			var titleCut, volumeCut bool
			title, titleCut = boundedCopilotText(title, 40)
			volume, volumeCut = boundedCopilotText(volume, 30)
			chapterBudget -= len([]rune(title)) + len([]rune(volume))
			truncated = truncated || titleCut || volumeCut
			chapters = append(chapters, map[string]any{"id": id, "title": title, "volume": volume, "order": order, "revisionId": revision, "finalRevisionId": final, "titleTruncated": titleCut, "volumeTruncated": volumeCut})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return CopilotToolResult{}, err
		}
		// The full brief is persisted as author data, while a bounded projection goes
		// to the model. A long plot plan remains retrievable from the UI unchanged.
		projected := brief
		var briefTruncated bool
		plot := []rune(brief.Data.PlotPlan)
		if page.PlotOffset > len(plot) {
			return CopilotToolResult{}, fmt.Errorf("规划续读超出正文范围")
		}
		pagedBrief := brief.Data
		pagedBrief.PlotPlan = string(plot[page.PlotOffset:])
		projected.Data, briefTruncated = boundedBookBrief(pagedBrief)
		truncated = truncated || briefTruncated
		description, descriptionTruncated := boundedCopilotText(project.Description, 200)
		title, titleCut := boundedCopilotText(project.Title, 80)
		genre, genreCut := boundedCopilotText(project.Genre, 40)
		truncated = truncated || descriptionTruncated || titleCut || genreCut
		out := CopilotToolResult{Content: map[string]any{"source": "author-book-planning", "project": map[string]any{"id": project.ID, "title": title, "description": description, "genre": genre, "epoch": project.Epoch}, "brief": projected, "settings": settings, "chapters": chapters, "plotOffset": page.PlotOffset, "nextPlotOffset": page.PlotOffset + len([]rune(projected.Data.PlotPlan)), "totalPlotChars": len(plot), "chapterOffset": page.ChapterOffset, "nextChapterOffset": page.ChapterOffset + len(chapters), "chapterLimit": page.ChapterLimit, "chapterHasMore": chapterHasMore, "truncated": truncated, "authority": "全书规划与作者设定属于创作约束和未来计划，不证明事件已发生；未读取任何章节正文。截断时按偏移续读，设定可用read_settings定向查找，不猜测省略内容。"}}
		return out, tx.Commit(ctx)
	case "read_settings":
		out, err := readBookSettings(ctx, tx, t, args)
		if err != nil {
			return CopilotToolResult{}, err
		}
		return out, tx.Commit(ctx)
	case "search_story_memory":
		out, err := searchBookMemory(ctx, tx, t, args)
		if err != nil {
			return CopilotToolResult{}, err
		}
		return out, tx.Commit(ctx)
	case "propose_book_plan":
		var input struct {
			Text  string `json:"text"`
			Title string `json:"title"`
		}
		if err = copilotDecode(args, &input); err != nil {
			return CopilotToolResult{}, err
		}
		if strings.TrimSpace(input.Text) == "" || len([]rune(input.Text)) > 40000 || len(input.Title) > 480 || callID == "" || len(callID) > 250 {
			return CopilotToolResult{}, fmt.Errorf("全书候选内容、标题或工具调用ID无效")
		}
		if input.Title == "" {
			input.Title = "全书规划候选"
		}
		raw, _ := json.Marshal(input)
		hash := job.Hash(raw)
		id := job.NewID()
		expected := t.Input.StudioVersions["brief"]
		frozen, _ := json.Marshal(t.Input)
		tag, err := tx.Exec(ctx, "INSERT INTO wb_book_plan_proposals(id,project_id,owner_id,turn_id,tool_call_id,input_hash,input,title,text,expected_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(turn_id,tool_call_id) DO NOTHING", id, t.ProjectID, t.OwnerID, t.ID, callID, hash, frozen, input.Title, input.Text, expected)
		if err != nil {
			return CopilotToolResult{}, err
		}
		if tag.RowsAffected() == 0 {
			var oldHash string
			if err = tx.QueryRow(ctx, "SELECT id,input_hash FROM wb_book_plan_proposals WHERE turn_id=$1 AND tool_call_id=$2", t.ID, callID).Scan(&id, &oldHash); err != nil {
				return CopilotToolResult{}, err
			}
			if hash != oldHash {
				return CopilotToolResult{}, &Conflict{Message: "工具调用已保存不同的全书方案"}
			}
		}
		artifact := &CopilotArtifact{Kind: "book-plan-proposal", ProposalID: id, Title: input.Title, Text: input.Text, ExpectedVersion: expected, BlueprintVersion: expected}
		out := CopilotToolResult{Artifact: artifact, Content: map[string]any{"proposalId": id, "expectedVersion": expected, "status": "awaiting-author-adoption", "text": input.Text, "authority": "只保存独立全书方案；作者明确采用前，书籍设定、正文和确认事实均未改变。"}}
		return out, tx.Commit(ctx)
	default:
		return CopilotToolResult{}, fmt.Errorf("全书规划仅可使用全书规划工具；章节写作请切换章节会话")
	}
}

func readBookSettings(ctx context.Context, tx pgx.Tx, t CopilotTurn, raw json.RawMessage) (CopilotToolResult, error) {
	return readCopilotSettings(ctx, tx, t.ProjectID, t.Input.Epoch, "", raw, 1000)
}

func readCopilotSettings(ctx context.Context, db queryer, pid string, epoch int64, pov string, raw json.RawMessage, itemLimit int) (CopilotToolResult, error) {
	var args struct {
		ID            string `json:"id"`
		Kind          string `json:"kind"`
		Query         string `json:"query"`
		Offset        int    `json:"offset"`
		ContentOffset int    `json:"contentOffset"`
	}
	if err := copilotDecode(raw, &args); err != nil {
		return CopilotToolResult{}, err
	}
	if len([]rune(args.Query)) > 500 || args.Offset < 0 || args.Offset > 1000000 || args.ContentOffset < 0 || (args.ContentOffset > 0 && args.ID == "") {
		return CopilotToolResult{}, fmt.Errorf("设定检索或续读位置无效")
	}
	if args.Kind != "" && args.Kind != "character" && args.Kind != "world" && args.Kind != "rule" && args.Kind != "style" && args.Kind != "outline" {
		return CopilotToolResult{}, fmt.Errorf("设定类型无效")
	}
	if pov != "" && args.ContentOffset != 0 {
		return CopilotToolResult{}, ErrForbidden
	}
	rows, err := db.Query(ctx, `SELECT id,project_id,kind,name,content FROM wb_settings WHERE project_id=$1 AND ($2='' OR id=$2) AND ($3='' OR kind=$3) AND ($4='' OR strpos(lower(name||CASE WHEN $6 THEN '' ELSE ' '||content END),lower($4))>0) ORDER BY kind,name,id LIMIT 9 OFFSET $5`, pid, args.ID, args.Kind, args.Query, args.Offset, pov != "")
	if err != nil {
		return CopilotToolResult{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	truncated := false
	budget := 3600
	for rows.Next() {
		var item Setting
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Kind, &item.Name, &item.Content); err != nil {
			return CopilotToolResult{}, err
		}
		if len(items) == 8 || budget <= 0 {
			truncated = true
			continue
		}
		if pov != "" {
			item.Content = ""
		}
		runes := []rune(item.Content)
		if args.ContentOffset > len(runes) {
			return CopilotToolResult{}, fmt.Errorf("设定续读超出正文范围")
		}
		text, cut := boundedCopilotText(string(runes[args.ContentOffset:]), min(itemLimit, budget))
		name, _ := boundedCopilotText(item.Name, 80)
		budget -= len([]rune(text))
		items = append(items, map[string]any{"id": item.ID, "projectId": pid, "kind": item.Kind, "name": name, "content": text, "contentOffset": args.ContentOffset, "nextContentOffset": args.ContentOffset + len([]rune(text)), "totalChars": len(runes), "truncated": cut})
	}
	if err := rows.Err(); err != nil {
		return CopilotToolResult{}, err
	}
	return CopilotToolResult{Content: map[string]any{"source": "author-settings", "projectId": pid, "epoch": epoch, "settings": items, "offset": args.Offset, "nextOffset": args.Offset + len(items), "truncated": truncated, "pointOfViewCharacterId": pov, "authority": "作者设定属于创作约束；未来计划不等于已发生事实，角色设定不等于角色已经获知。角色视角只提供名称标识，不提供全知内容。条目截断时按id和nextContentOffset续读，不从下一条的offset跳过本条未读内容。"}}, nil
}

func searchBookMemory(ctx context.Context, tx pgx.Tx, t CopilotTurn, raw json.RawMessage) (CopilotToolResult, error) {
	var args struct {
		Query       string `json:"query"`
		Limit       int    `json:"limit"`
		Offset      int    `json:"offset"`
		FactID      string `json:"factId"`
		Field       string `json:"field"`
		FieldOffset int    `json:"fieldOffset"`
	}
	if err := copilotDecode(raw, &args); err != nil {
		return CopilotToolResult{}, err
	}
	if (args.FactID == "" && strings.TrimSpace(args.Query) == "") || len([]rune(args.Query)) > 500 || args.Offset < 0 || args.Offset > 1000000 || args.FieldOffset < 0 || (args.FactID == "" && (args.Field != "" || args.FieldOffset != 0)) {
		return CopilotToolResult{}, fmt.Errorf("请输入不超过500字的查询")
	}
	if args.Limit == 0 {
		args.Limit = 6
	}
	if args.Limit < 1 || args.Limit > 8 {
		return CopilotToolResult{}, fmt.Errorf("检索条数应为1至8")
	}
	facts, err := factsAt(ctx, tx, t.ProjectID, 2147483647)
	if err != nil {
		return CopilotToolResult{}, err
	}
	if args.FactID != "" {
		for _, fact := range facts {
			if fact.ID != args.FactID {
				continue
			}
			field := args.Field
			if field == "" {
				field = "object"
			}
			text := fact.Object
			if field == "quote" {
				text = fact.Evidence.Quote
			} else if field != "object" {
				return CopilotToolResult{}, fmt.Errorf("事实字段无效")
			}
			runes := []rune(text)
			if args.FieldOffset > len(runes) {
				return CopilotToolResult{}, fmt.Errorf("事实续读超出范围")
			}
			end := min(len(runes), args.FieldOffset+2000)
			return CopilotToolResult{Content: map[string]any{"source": "current-confirmed-book-fact-field", "id": fact.ID, "kind": fact.Kind, "characterId": fact.CharacterID, "truthStatus": fact.TruthStatus, "sourceRevisionId": fact.SourceRevisionID, "sourceHash": fact.Evidence.PlainTextHash, "field": field, "text": string(runes[args.FieldOffset:end]), "offset": args.FieldOffset, "nextOffset": end, "totalChars": len(runes), "truncated": end < len(runes) || args.FieldOffset > 0, "authority": "当前有效确认事实的来源分页；Knowledge保留特定角色获知/误信，不变成全知真相。"}}, nil
		}
		return CopilotToolResult{}, ErrNotFound
	}
	type match struct {
		fact   Fact
		score  float64
		reason string
	}
	matches := []match{}
	terms := tokenize(args.Query)
	for _, fact := range facts {
		score, reason := relevance(fact.Subject+" "+fact.Predicate+" "+fact.Object, args.Query, terms, nil)
		if score > 0 {
			matches = append(matches, match{fact, score, reason})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].fact.ID < matches[j].fact.ID
	})
	if args.Offset > len(matches) {
		return CopilotToolResult{}, fmt.Errorf("检索结果续读超出范围")
	}
	end := min(len(matches), args.Offset+min(args.Limit, 6))
	more := end < len(matches)
	matches = matches[args.Offset:end]
	citations := []map[string]any{}
	for _, match := range matches {
		f := match.fact
		object, cut := boundedCopilotText(f.Object, 200)
		quote, quoteCut := boundedCopilotText(f.Evidence.Quote, 100)
		subject, _ := boundedCopilotText(f.Subject, 40)
		predicate, _ := boundedCopilotText(f.Predicate, 60)
		citations = append(citations, map[string]any{"id": f.ID, "kind": f.Kind, "subject": subject, "predicate": predicate, "object": object, "characterId": f.CharacterID, "truthStatus": f.TruthStatus, "source": map[string]any{"chapterId": f.ChapterID, "chapterOrder": f.ChapterOrder, "revisionId": f.SourceRevisionID, "hash": f.Evidence.PlainTextHash, "quote": quote}, "score": match.score, "fieldReadTool": "search_story_memory", "truncated": cut || quoteCut})
	}
	return CopilotToolResult{Content: map[string]any{"source": "current-confirmed-book-facts", "purpose": "AuthorPlanning", "stateVersionId": t.Input.StateVersionID, "citations": citations, "offset": args.Offset, "nextOffset": end, "hasMore": more, "truncated": more, "authority": "offset续读检索结果；被截断事实用factId/field/fieldOffset读取完整来源字段。仅返回当前有效定稿来源中作者已确认的事实；未读取整本正文，未确认候选不包含在内。Knowledge是特定角色的获知或误信，不能当成客观真相或其他角色共同知识。"}}, nil
}

func (s *Service) ApplyCopilotBookPlan(ctx context.Context, id string, expected int64) (StudioDocument[StoryBrief], error) {
	var pid, owner, turnID, title, text string
	var baseline int64
	var inputRaw []byte
	err := s.DB.QueryRow(ctx, "SELECT project_id,owner_id,turn_id,title,text,expected_version,input FROM wb_book_plan_proposals WHERE id=$1", id).Scan(&pid, &owner, &turnID, &title, &text, &baseline, &inputRaw)
	if err != nil {
		return StudioDocument[StoryBrief]{}, noRows(err)
	}
	if owner != copilotOwner(ctx) {
		return StudioDocument[StoryBrief]{}, ErrForbidden
	}
	if err = s.RequireProjectRole(ctx, pid, "editor"); err != nil {
		return StudioDocument[StoryBrief]{}, err
	}
	if expected != baseline {
		return StudioDocument[StoryBrief]{}, &Conflict{Message: "全书规划版本与候选基线不一致"}
	}
	var sourceStatus string
	var sourceStale bool
	if err = s.DB.QueryRow(ctx, "SELECT status,stale FROM wb_copilot_turns WHERE id=$1", turnID).Scan(&sourceStatus, &sourceStale); err != nil {
		return StudioDocument[StoryBrief]{}, err
	}
	if sourceStatus != "Succeeded" || sourceStale {
		return StudioDocument[StoryBrief]{}, &Conflict{Message: "全书方案来源对话尚未成功完成或已失效"}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return StudioDocument[StoryBrief]{}, err
	}
	defer tx.Rollback(ctx)
	var projectID string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR UPDATE", pid).Scan(&projectID); err != nil {
		return StudioDocument[StoryBrief]{}, noRows(err)
	}
	var adopted *int64
	if err = tx.QueryRow(ctx, "SELECT adopted_version FROM wb_book_plan_proposals WHERE id=$1 FOR UPDATE", id).Scan(&adopted); err != nil {
		return StudioDocument[StoryBrief]{}, err
	}
	doc, err := studioRead(ctx, tx, pid, "brief", "main", StoryBrief{WorldRules: []string{}, Volumes: []StoryVolume{}})
	if err != nil {
		return doc, err
	}
	if adopted != nil {
		if doc.Version != *adopted {
			return doc, &Conflict{Message: "方案已采用，之后全书规划又有更新，请查看历史"}
		}
		return doc, tx.Commit(ctx)
	}
	var input CopilotInput
	if err = json.Unmarshal(inputRaw, &input); err != nil {
		return doc, err
	}
	valid, err := copilotSnapshotCurrent(ctx, tx, CopilotTurn{ProjectID: pid, Input: input})
	if err != nil {
		return doc, err
	}
	if !valid {
		return doc, &Conflict{Message: "全书设定、规划或作品基线已变化，请重新讨论方案"}
	}
	var status string
	var stale bool
	if err = tx.QueryRow(ctx, "SELECT status,stale FROM wb_copilot_turns WHERE id=$1 FOR SHARE", turnID).Scan(&status, &stale); err != nil {
		return doc, err
	}
	if stale || status != "Succeeded" {
		return doc, &Conflict{Message: "全书方案来源对话尚未成功完成或已失效"}
	}
	doc.Data.PlotPlan = text
	saved, err := studioSaveTx(ctx, tx, pid, "brief", "main", expected, doc.Data)
	if err != nil {
		return saved, err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_book_plan_proposals SET adopted_version=$2 WHERE id=$1", id, saved.Version); err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}
