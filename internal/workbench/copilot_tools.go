package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

// CopilotToolContext is populated by the host from the persisted turn snapshot,
// never from model tool arguments. Every tool rechecks this baseline and access.
type CopilotToolContext struct {
	ProjectID              string                `json:"projectId"`
	ChapterID              string                `json:"chapterId"`
	RevisionID             string                `json:"revisionId"`
	Epoch                  int64                 `json:"epoch"`
	StateVersionID         string                `json:"stateVersionId"`
	BlueprintVersion       int64                 `json:"blueprintVersion"`
	StudioVersions         map[string]int64      `json:"studioVersions"`
	WritingSkills          *WritingSkillSnapshot `json:"writingSkills,omitempty"`
	Selection              string                `json:"selection"`
	PointOfViewCharacterID string                `json:"pointOfViewCharacterId,omitempty"`
	Principal              Principal             `json:"principal"`
	TurnID                 string                `json:"turnId"`
	CallID                 string                `json:"callId,omitempty"`
}
type CopilotToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
type CopilotDiffHunk struct {
	ID       int    `json:"id"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Changed  bool   `json:"changed"`
	Accepted bool   `json:"accepted"`
}
type CopilotArtifact struct {
	Kind             string            `json:"kind"`
	Title            string            `json:"title"`
	RunID            string            `json:"runId,omitempty"`
	ProposalID       string            `json:"proposalId,omitempty"`
	ExpectedVersion  int64             `json:"expectedVersion"`
	ChapterID        string            `json:"chapterId"`
	SourceRevisionID string            `json:"sourceRevisionId"`
	SourceHash       string            `json:"sourceHash"`
	Text             string            `json:"text,omitempty"`
	BlueprintVersion int64             `json:"blueprintVersion"`
	Diff             []CopilotDiffHunk `json:"diff,omitempty"`
}
type CopilotToolResult struct {
	Content  any              `json:"content"`
	Artifact *CopilotArtifact `json:"artifact,omitempty"`
}

func (s *Service) CopilotTools() []CopilotToolDefinition {
	return []CopilotToolDefinition{
		{"read_chapter", "分页读冻结本章正文及来源版本，offset=nextOffset。章纲/选段另读对应工具。", json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer","minimum":0},"maxChars":{"type":"integer","minimum":200,"maximum":4000}},"additionalProperties":false}`)},
		{"read_selection", "分页读完整选段，offset=nextOffset；改稿前读全。", json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer","minimum":0},"maxChars":{"type":"integer","minimum":200,"maximum":4000}},"additionalProperties":false}`)},
		{"read_blueprint", "分页读冻结章纲，offset=nextOffset。角色视角拒绝；计划不是事实。", json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer","minimum":0},"maxChars":{"type":"integer","minimum":200,"maximum":4000}},"additionalProperties":false}`)},
		{"read_settings", "查设定，offset翻页，id/contentOffset续读；角色视角只按名称匹配且只返标识。", json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","enum":["","character","world","rule","style","outline"]},"query":{"type":"string","maxLength":500},"id":{"type":"string"},"offset":{"type":"integer","minimum":0},"contentOffset":{"type":"integer","minimum":0}},"additionalProperties":false}`)},
		{"search_story_memory", "查前章有效定稿，offset续页；characterId仅该角色知识，无旁白证据。", json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","maxLength":2000},"offset":{"type":"integer","minimum":0},"characterId":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":8}},"required":["query"],"additionalProperties":false}`)},
		{"check_consistency", "本地检查/确认事实；offset/issueOffset翻页，factId/field/fieldOffset续读，不执行模型审稿。", json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer","minimum":0},"issueOffset":{"type":"integer","minimum":0},"factId":{"type":"string"},"field":{"type":"string","enum":["object","quote"]},"fieldOffset":{"type":"integer","minimum":0}},"additionalProperties":false}`)},
		{"create_revision_candidate", "保存独立改稿候选/Diff；selection替换完整选段，chapter为整章。作者采用才写正文。", json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","minLength":1,"maxLength":60000},"mode":{"type":"string","enum":["chapter","selection"]},"title":{"type":"string","maxLength":120}},"required":["text","mode"],"additionalProperties":false}`)},
		{"propose_blueprint", "保存本章章纲独立候选，需作者采用；不写正文或事实。", json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","minLength":1,"maxLength":60000},"title":{"type":"string","maxLength":120}},"required":["text"],"additionalProperties":false}`)},
		{"show_diff", "比较本章runId/text与原稿，返回Diff，不改正文。", json.RawMessage(`{"type":"object","properties":{"runId":{"type":"string"},"text":{"type":"string","maxLength":60000}},"additionalProperties":false}`)},
	}
}

func (s *Service) NewCopilotToolContext(ctx context.Context, turnID, pid, cid, revisionID, selection string, pointOfView ...string) (CopilotToolContext, error) {
	p, ok := PrincipalFromContext(ctx)
	if !ok || p.ID == "" {
		return CopilotToolContext{}, ErrForbidden
	}
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return CopilotToolContext{}, err
	}
	c, err := s.GetChapter(ctx, cid)
	if err != nil {
		return CopilotToolContext{}, err
	}
	if c.ProjectID != pid {
		return CopilotToolContext{}, ErrNotFound
	}
	if revisionID == "" {
		revisionID = c.CurrentRevisionID
	}
	if c.CurrentRevisionID != revisionID {
		return CopilotToolContext{}, &Conflict{Message: "正文已变化，请重新发送", CurrentRevisionID: c.CurrentRevisionID}
	}
	r, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return CopilotToolContext{}, err
	}
	if r.ChapterID != cid {
		return CopilotToolContext{}, ErrNotFound
	}
	if len(selection) > 60000 || (selection != "" && strings.Count(r.PlainText, selection) != 1) {
		return CopilotToolContext{}, fmt.Errorf("选段须唯一对应当前保存正文")
	}
	d, err := s.GetProject(ctx, pid)
	if err != nil {
		return CopilotToolContext{}, err
	}
	studio, err := s.StudioPromptContext(ctx, pid)
	if err != nil {
		return CopilotToolContext{}, err
	}
	skills, err := s.ResolveWritingSkills(ctx, pid, "copilot")
	if err != nil {
		return CopilotToolContext{}, err
	}
	out := CopilotToolContext{ProjectID: pid, ChapterID: cid, RevisionID: revisionID, Epoch: d.Project.Epoch, StateVersionID: d.Project.StateVersionID, BlueprintVersion: c.BlueprintVersion, StudioVersions: studio.Versions, WritingSkills: &skills, Selection: selection, Principal: p, TurnID: turnID}
	if len(pointOfView) > 0 {
		out.PointOfViewCharacterID = pointOfView[0]
	}
	if out.PointOfViewCharacterID != "" {
		var valid bool
		err = s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_settings WHERE id=$1 AND project_id=$2 AND kind='character')", out.PointOfViewCharacterID, pid).Scan(&valid)
		if err != nil {
			return out, err
		}
		if !valid {
			return out, fmt.Errorf("视角人物不属于本作品")
		}
	}
	_, _, err = s.copilotToolBaseline(ctx, out, "viewer")
	return out, err
}

func copilotDecode(raw json.RawMessage, value any) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if len(raw) > 250000 {
		return fmt.Errorf("工具参数超过大小限制")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("工具参数不符合声明的Schema")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("工具参数必须为单个JSON对象")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("工具参数必须为JSON对象")
	}
	return nil
}
func (scope CopilotToolContext) runInput(source Revision, chapter Chapter) RunInput {
	return RunInput{ProjectID: scope.ProjectID, ChapterID: scope.ChapterID, RevisionID: scope.RevisionID, Epoch: scope.Epoch, StateVersionID: scope.StateVersionID, Blueprint: chapter.Blueprint, BlueprintVersion: scope.BlueprintVersion, Source: source.PlainText, SourceHash: source.Hash, StudioVersions: scope.StudioVersions, WritingSkills: writingSkillBaseline(scope.WritingSkills), Selection: scope.Selection, Provider: "copilot", Model: "included-in-copilot-attempt"}
}
func (s *Service) copilotToolBaseline(ctx context.Context, scope CopilotToolContext, minimum string) (Chapter, Revision, error) {
	if scope.Principal.ID == "" || scope.ProjectID == "" || scope.ChapterID == "" || scope.RevisionID == "" {
		return Chapter{}, Revision{}, ErrForbidden
	}
	ctx = WithPrincipal(ctx, scope.Principal)
	if err := s.RequireProjectRole(ctx, scope.ProjectID, minimum); err != nil {
		return Chapter{}, Revision{}, err
	}
	c, err := s.GetChapter(ctx, scope.ChapterID)
	if err != nil {
		return c, Revision{}, err
	}
	if c.ProjectID != scope.ProjectID {
		return Chapter{}, Revision{}, ErrNotFound
	}
	r, err := s.GetRevision(ctx, scope.RevisionID)
	if err != nil {
		return c, r, err
	}
	if r.ChapterID != c.ID {
		return c, r, ErrNotFound
	}
	valid, err := snapshotCurrent(ctx, s.DB, Run{ChapterID: c.ID, Type: "revision", Input: scope.runInput(r, c)})
	if err != nil {
		return c, r, err
	}
	if !valid {
		return c, r, &Conflict{Message: "正文、章纲或作品基线已变化，请重新发送", CurrentRevisionID: c.CurrentRevisionID}
	}
	return c, r, nil
}
func boundedCopilotText(text string, max int) (string, bool) {
	r := []rune(text)
	if len(r) > max {
		return string(r[:max]), true
	}
	return text, false
}

func (s *Service) ExecuteCopilotTool(ctx context.Context, scope CopilotToolContext, name string, arguments json.RawMessage) (CopilotToolResult, error) {
	minimum := "viewer"
	if name == "create_revision_candidate" || name == "propose_blueprint" {
		minimum = "editor"
	}
	ctx = WithPrincipal(ctx, scope.Principal)
	c, r, err := s.copilotToolBaseline(ctx, scope, minimum)
	if err != nil {
		return CopilotToolResult{}, err
	}
	result := CopilotToolResult{}
	switch name {
	case "read_writing_skill":
		result, err = readFrozenWritingSkill(scope.WritingSkills, arguments)
		if err != nil {
			return result, err
		}
	case "read_selection", "read_blueprint":
		var args struct {
			Offset   int `json:"offset"`
			MaxChars int `json:"maxChars"`
		}
		if err = copilotDecode(arguments, &args); err != nil {
			return result, err
		}
		if args.MaxChars == 0 {
			args.MaxChars = 2000
		}
		if args.Offset < 0 || args.MaxChars < 200 || args.MaxChars > 4000 {
			return result, fmt.Errorf("来源续读位置或长度无效")
		}
		text := scope.Selection
		source := "author-selection"
		if name == "read_blueprint" {
			if scope.PointOfViewCharacterID != "" {
				return result, ErrForbidden
			}
			text = c.Blueprint
			source = "author-blueprint-plan"
		}
		runes := []rune(text)
		if args.Offset > len(runes) {
			return result, fmt.Errorf("来源续读超出正文范围")
		}
		end := min(len(runes), args.Offset+args.MaxChars)
		result.Content = map[string]any{"source": source, "chapterId": c.ID, "revisionId": r.ID, "hash": r.Hash, "blueprintVersion": c.BlueprintVersion, "text": string(runes[args.Offset:end]), "offset": args.Offset, "nextOffset": end, "totalChars": len(runes), "sourceCharCount": len(runes), "sourceTextHash": job.Hash([]byte(text)), "truncated": end < len(runes) || args.Offset > 0, "authority": "作者选段属于已保存正文，章纲属于未来计划。分页仅是资料投影，不改变原始选段锚点；替换选段需理解完整选段，不把未读部分当作不存在。"}
		if name == "read_selection" {
			result.Content.(map[string]any)["selectionHash"] = job.Hash([]byte(scope.Selection))
		}
	case "read_chapter":
		var args struct {
			Offset   int `json:"offset"`
			MaxChars int `json:"maxChars"`
		}
		if err = copilotDecode(arguments, &args); err != nil {
			return result, err
		}
		if args.MaxChars == 0 {
			args.MaxChars = 4000
		}
		if args.MaxChars < 200 || args.MaxChars > 4000 || args.Offset < 0 {
			return result, fmt.Errorf("读取范围无效")
		}
		text := []rune(r.PlainText)
		if args.Offset > len(text) {
			args.Offset = len(text)
		}
		end := args.Offset + args.MaxChars
		if end > len(text) {
			end = len(text)
		}
		blueprint, bpTruncated := "", c.Blueprint != ""
		if scope.PointOfViewCharacterID != "" {
			blueprint = "角色视角模式不提供全知章纲。"
			bpTruncated = false
		}
		selection, selectionTruncated := "", scope.Selection != ""
		chapterTitle, titleTruncated := boundedCopilotText(c.Title, 60)
		result.Content = map[string]any{"source": "author-saved-revision", "chapterId": c.ID, "chapterTitle": chapterTitle, "chapterTitleTruncated": titleTruncated, "chapterOrder": c.Order, "revisionId": r.ID, "hash": r.Hash, "text": string(text[args.Offset:end]), "offset": args.Offset, "nextOffset": end, "totalChars": len(text), "truncated": end < len(text) || args.Offset > 0, "blueprint": blueprint, "blueprintTruncated": bpTruncated, "blueprintNextOffset": len([]rune(blueprint)), "blueprintReadTool": "read_blueprint", "blueprintVersion": c.BlueprintVersion, "selection": selection, "selectionTruncated": selectionTruncated, "selectionNextOffset": len([]rune(selection)), "selectionReadTool": "read_selection", "authority": "正文offset续读；选段/章纲另读，计划非事实。"}
	case "read_settings":
		result, err = readCopilotSettings(ctx, s.DB, scope.ProjectID, scope.Epoch, scope.PointOfViewCharacterID, arguments, 2000)
		if err != nil {
			return result, err
		}
	case "search_story_memory":
		var args struct {
			Query       string `json:"query"`
			CharacterID string `json:"characterId"`
			Limit       int    `json:"limit"`
			Offset      int    `json:"offset"`
		}
		if err = copilotDecode(arguments, &args); err != nil {
			return result, err
		}
		if len(args.Query) > 2000 || strings.TrimSpace(args.Query) == "" {
			return result, fmt.Errorf("请输入不超过2000字节的查询")
		}
		if args.Limit == 0 {
			args.Limit = 6
		}
		if args.Limit < 1 || args.Limit > 8 || args.Offset < 0 || args.Offset > 50 {
			return result, fmt.Errorf("检索条数应为1至8")
		}
		if scope.PointOfViewCharacterID != "" {
			if args.CharacterID != "" && args.CharacterID != scope.PointOfViewCharacterID {
				return result, ErrForbidden
			}
			args.CharacterID = scope.PointOfViewCharacterID
		}
		purpose := "AuthorReview"
		if args.CharacterID != "" {
			purpose = "CharacterKnowledge"
		}
		citations, err := s.Search(ctx, Query{ProjectID: scope.ProjectID, ProjectEpoch: scope.Epoch, BeforeChapterID: scope.ChapterID, Text: args.Query, CharacterID: args.CharacterID, Purpose: purpose, Limit: 50})
		if err != nil {
			return result, err
		}
		if args.Offset > len(citations) {
			return result, fmt.Errorf("检索续读超出范围")
		}
		allCount := len(citations)
		end := min(allCount, args.Offset+min(args.Limit, 6))
		citations = citations[args.Offset:end]
		truncated := false
		contentBudget := 2400
		for i := range citations {
			var cut bool
			citations[i].Content, cut = boundedCopilotText(citations[i].Content, min(400, contentBudget))
			contentBudget -= len([]rune(citations[i].Content))
			truncated = truncated || cut
			citations[i].ChapterTitle, _ = boundedCopilotText(citations[i].ChapterTitle, 60)
			citations[i].Reason, _ = boundedCopilotText(citations[i].Reason, 80)
		}
		result.Content = map[string]any{"source": "current-confirmed-before-chapter", "purpose": purpose, "beforeChapterId": c.ID, "citations": citations, "offset": args.Offset, "nextOffset": end, "hasMore": end < allCount, "truncated": truncated || end < allCount, "sourceReadTool": "read_chapter", "authority": "offset续读最多50条检索结果，引用片段截断时依据当前Revision与来源在页面查看完整原文。只含当前章之前的有效来源；Knowledge保留获知者和误信状态，不等于客观真相。"}
	case "check_consistency":
		result, err = s.copilotConsistency(ctx, scope, c, r, arguments)
		if err != nil {
			return result, err
		}
	case "create_revision_candidate", "propose_blueprint":
		var args struct {
			Text  string `json:"text"`
			Mode  string `json:"mode"`
			Title string `json:"title"`
		}
		if err = copilotDecode(arguments, &args); err != nil {
			return result, err
		}
		if strings.TrimSpace(args.Text) == "" || utf8.RuneCountInString(args.Text) > 60000 || len(args.Text) > 200000 || len(args.Title) > 480 {
			return result, fmt.Errorf("候选内容或标题超过限制")
		}
		kind := "revision"
		text := args.Text
		if name == "propose_blueprint" {
			kind = "planning"
			if args.Mode != "" {
				return result, fmt.Errorf("章纲提议不接受正文模式")
			}
		} else {
			switch args.Mode {
			case "chapter":
			case "selection":
				if scope.Selection == "" || strings.Count(r.PlainText, scope.Selection) != 1 {
					return result, fmt.Errorf("本轮没有唯一作者选段")
				}
				text = strings.Replace(r.PlainText, scope.Selection, args.Text, 1)
			default:
				return result, fmt.Errorf("候选模式须为chapter或selection")
			}
		}
		run, err := s.newCopilotCandidate(ctx, scope, c, r, kind, text)
		if err != nil {
			return result, err
		}
		artifactKind := "revision-candidate"
		title := strings.TrimSpace(args.Title)
		if kind == "planning" {
			artifactKind = "blueprint-proposal"
		}
		if title == "" {
			if kind == "planning" {
				title = "本章章纲建议"
			} else {
				title = "正文修改候选"
			}
		}
		artifact := &CopilotArtifact{Kind: artifactKind, Title: title, RunID: run.ID, ChapterID: c.ID, SourceRevisionID: r.ID, SourceHash: r.Hash, Text: run.Output.Text, BlueprintVersion: c.BlueprintVersion}
		if kind == "revision" {
			artifact.Diff = copilotParagraphDiff(r.PlainText, run.Output.Text)
		}
		result.Artifact = artifact
		result.Content = map[string]any{"runId": run.ID, "kind": artifact.Kind, "status": "awaiting-author-adoption", "sourceRevisionId": r.ID, "blueprintVersion": c.BlueprintVersion, "text": run.Output.Text, "authority": "候选已保存，正文、章纲和确认事实均未改变；作者需比较并明确采用。"}
	case "show_diff":
		var args struct {
			RunID string `json:"runId"`
			Text  string `json:"text"`
		}
		if err = copilotDecode(arguments, &args); err != nil {
			return result, err
		}
		if (args.RunID == "") == (args.Text == "") {
			return result, fmt.Errorf("需提供候选runId或text其中一项")
		}
		text := args.Text
		if utf8.RuneCountInString(text) > 60000 {
			return result, fmt.Errorf("候选过长")
		}
		if args.RunID != "" {
			run, err := s.GetRun(ctx, args.RunID)
			if err != nil {
				return result, err
			}
			if run.ProjectID != scope.ProjectID || run.ChapterID != c.ID {
				return result, ErrNotFound
			}
			if run.Status != "Succeeded" || run.Stale || run.Input.RevisionID != r.ID || (run.Type != "revision" && run.Type != "draft" && run.Type != "rewrite") {
				return result, &Conflict{Message: "候选来源已失效或不是正文候选"}
			}
			valid, err := snapshotCurrent(ctx, s.DB, run)
			if err != nil {
				return result, err
			}
			if !valid {
				return result, &Conflict{Message: "候选输入已经变化"}
			}
			text = run.Output.Text
		}
		hunks := copilotParagraphDiff(r.PlainText, text)
		result.Artifact = &CopilotArtifact{Kind: "diff", Title: "正文差异", RunID: args.RunID, ChapterID: c.ID, SourceRevisionID: r.ID, SourceHash: r.Hash, Text: text, BlueprintVersion: c.BlueprintVersion, Diff: hunks}
		result.Content = map[string]any{"sourceRevisionId": r.ID, "sourceHash": r.Hash, "diff": hunks, "authority": "真实段落差异，不采用、不覆盖正文。"}
	default:
		return result, fmt.Errorf("未授权或不存在的小说工具")
	}
	// Reads must not finish with old visibility after concurrent revision/memory
	// invalidation. Mutating candidate creation also checked within its transaction.
	if _, _, err = s.copilotToolBaseline(ctx, scope, minimum); err != nil {
		return CopilotToolResult{}, err
	}
	return result, nil
}

func (s *Service) copilotConsistency(ctx context.Context, scope CopilotToolContext, c Chapter, r Revision, raw json.RawMessage) (CopilotToolResult, error) {
	var args struct {
		Offset      int    `json:"offset"`
		IssueOffset int    `json:"issueOffset"`
		FactID      string `json:"factId"`
		Field       string `json:"field"`
		FieldOffset int    `json:"fieldOffset"`
	}
	if err := copilotDecode(raw, &args); err != nil {
		return CopilotToolResult{}, err
	}
	if args.Offset < 0 || args.IssueOffset < 0 || args.FieldOffset < 0 || args.Offset > 1000000 || args.IssueOffset > 1000000 {
		return CopilotToolResult{}, fmt.Errorf("检查续读位置无效")
	}
	if args.FactID == "" && (args.Field != "" || args.FieldOffset != 0) {
		return CopilotToolResult{}, fmt.Errorf("续读字段需要指定事实ID")
	}
	facts, err := factsAt(ctx, s.DB, scope.ProjectID, c.Order-1)
	if err != nil {
		return CopilotToolResult{}, err
	}
	visible := []Fact{}
	for _, fact := range facts {
		if scope.PointOfViewCharacterID == "" || (fact.Kind == "Knowledge" && fact.CharacterID == scope.PointOfViewCharacterID) {
			visible = append(visible, fact)
		}
	}
	if args.FactID != "" {
		for _, fact := range visible {
			if fact.ID != args.FactID {
				continue
			}
			field := args.Field
			if field == "" {
				field = "object"
			}
			text := fact.Object
			if field == "quote" {
				if scope.PointOfViewCharacterID != "" {
					return CopilotToolResult{}, ErrForbidden
				}
				text = fact.Evidence.Quote
			} else if field != "object" {
				return CopilotToolResult{}, fmt.Errorf("事实字段无效")
			}
			runes := []rune(text)
			if args.FieldOffset > len(runes) {
				return CopilotToolResult{}, fmt.Errorf("事实字段续读超出范围")
			}
			end := min(len(runes), args.FieldOffset+2000)
			return CopilotToolResult{Content: map[string]any{"source": "confirmed-memory-field", "id": fact.ID, "kind": fact.Kind, "characterId": fact.CharacterID, "truthStatus": fact.TruthStatus, "sourceRevisionId": fact.SourceRevisionID, "sourceHash": fact.Evidence.PlainTextHash, "field": field, "text": string(runes[args.FieldOffset:end]), "offset": args.FieldOffset, "nextOffset": end, "totalChars": len(runes), "truncated": end < len(runes) || args.FieldOffset > 0, "semanticReviewPerformed": false, "authority": "这是当前章之前的已确认来源分页；Knowledge仅代表对应角色获知或误信，不是全知真相。"}}, nil
		}
		return CopilotToolResult{}, ErrNotFound
	}
	if args.Offset > len(visible) {
		return CopilotToolResult{}, fmt.Errorf("事实续读超出范围")
	}
	memory := []map[string]any{}
	budget := 2000
	truncated := false
	for _, fact := range visible[args.Offset:] {
		if len(memory) == 8 || budget < 100 {
			break
		}
		predicate, predicateCut := boundedCopilotText(fact.Predicate, min(60, budget))
		budget -= len([]rune(predicate))
		object, objectCut := boundedCopilotText(fact.Object, min(200, budget))
		budget -= len([]rune(object))
		quote := ""
		quoteCut := false
		if scope.PointOfViewCharacterID == "" {
			quote, quoteCut = boundedCopilotText(fact.Evidence.Quote, min(120, budget))
			budget -= len([]rune(quote))
		}
		truncated = truncated || predicateCut || objectCut || quoteCut
		memory = append(memory, map[string]any{"id": fact.ID, "kind": fact.Kind, "subject": fact.Subject, "predicate": predicate, "object": object, "objectTotalChars": len([]rune(fact.Object)), "objectTruncated": objectCut, "quoteTruncated": quoteCut, "characterId": fact.CharacterID, "truthStatus": fact.TruthStatus, "source": map[string]any{"chapterId": fact.ChapterID, "chapterOrder": fact.ChapterOrder, "revisionId": fact.SourceRevisionID, "hash": fact.Evidence.PlainTextHash, "quote": quote}, "fieldReadTool": "check_consistency"})
	}
	factsNext := args.Offset + len(memory)
	issues, err := s.DeterministicReview(ctx, c.ID, r.ID)
	if err != nil {
		return CopilotToolResult{}, err
	}
	if scope.PointOfViewCharacterID != "" {
		filtered := issues[:0]
		for _, issue := range issues {
			if issue.Kind != "Rule" && issue.Kind != "Foreshadow" {
				filtered = append(filtered, issue)
			}
		}
		issues = filtered
	}
	if args.IssueOffset > len(issues) {
		return CopilotToolResult{}, fmt.Errorf("检查结果续读超出范围")
	}
	projected := []map[string]any{}
	issueBudget := 1200
	for _, issue := range issues[args.IssueOffset:] {
		if len(projected) == 4 || issueBudget < 100 {
			break
		}
		quote, qcut := boundedCopilotText(issue.Quote, min(120, issueBudget))
		issueBudget -= len([]rune(quote))
		explanation, ecut := boundedCopilotText(issue.Explanation, min(160, issueBudget))
		issueBudget -= len([]rune(explanation))
		action, acut := boundedCopilotText(issue.SuggestedAction, min(80, issueBudget))
		issueBudget -= len([]rune(action))
		quoteStart := -1
		if at := strings.Index(r.PlainText, issue.Quote); at >= 0 {
			quoteStart = len([]rune(r.PlainText[:at]))
		}
		projected = append(projected, map[string]any{"index": args.IssueOffset + len(projected), "kind": issue.Kind, "severity": issue.Severity, "quote": quote, "quoteOffset": quoteStart, "quoteTotalChars": len([]rune(issue.Quote)), "quoteTruncated": qcut, "explanation": explanation, "explanationTruncated": ecut, "suggestedAction": action, "suggestedActionTruncated": acut, "sourceReadTool": "read_chapter"})
		truncated = truncated || qcut || ecut || acut
	}
	issuesNext := args.IssueOffset + len(projected)
	return CopilotToolResult{Content: map[string]any{"source": "local-rules-and-confirmed-memory", "revisionId": r.ID, "hash": r.Hash, "issues": projected, "issueOffset": args.IssueOffset, "nextIssueOffset": issuesNext, "issuesHasMore": issuesNext < len(issues), "totalIssues": len(issues), "confirmedFacts": memory, "offset": args.Offset, "nextOffset": factsNext, "factsHasMore": factsNext < len(visible), "totalFacts": len(visible), "truncated": truncated || factsNext < len(visible) || issuesNext < len(issues), "semanticReviewPerformed": false, "authority": "本地检查不执行模型语义审稿。offset续读确认事实，issueOffset续读检查；截断事实用factId/field/fieldOffset继续取原字段，检查引句用read_chapter从quoteOffset读取。Knowledge保留角色获知及误信，不代表全知真相。"}}, nil
}

func (s *Service) newCopilotCandidate(ctx context.Context, scope CopilotToolContext, c Chapter, r Revision, kind, text string) (Run, error) {
	if scope.TurnID == "" {
		return Run{}, fmt.Errorf("候选缺少持久对话轮次")
	}
	if kind == "revision" {
		if _, _, _, err := NormalizeDocument(DocumentFromText(text)); err != nil {
			return Run{}, err
		}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	var pid string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR UPDATE", scope.ProjectID).Scan(&pid); err != nil {
		return Run{}, noRows(err)
	}
	// A real running turn is fenced in the same transaction as artifact creation.
	// Standalone tool fixtures may use an unpersisted turn id, but a persisted
	// cancelled/expired/foreign turn must never produce an additional candidate.
	var turnProject, turnOwner, turnStatus string
	var cancelled, leaseValid bool
	err = tx.QueryRow(ctx, "SELECT project_id,owner_id,status,cancel_requested,COALESCE(lease_until>now(),false) FROM wb_copilot_turns WHERE id=$1 FOR SHARE", scope.TurnID).Scan(&turnProject, &turnOwner, &turnStatus, &cancelled, &leaseValid)
	if err != nil && err != pgx.ErrNoRows {
		return Run{}, err
	}
	if err == nil && (turnProject != pid || turnOwner != scope.Principal.ID || turnStatus != "Running" || cancelled || !leaseValid) {
		return Run{}, &Conflict{Message: "对话已停止或工具执行租约失效"}
	}
	input := scope.runInput(r, c)
	input.TaskType = kind
	input.Scope = "chapter"
	input.Instructions = "Copilot对话中的作者审阅候选，计费已包含在对话调用中。"
	valid, err := snapshotCurrent(ctx, tx, Run{ChapterID: c.ID, Type: kind, Input: input})
	if err != nil {
		return Run{}, err
	}
	if !valid {
		return Run{}, &Conflict{Message: "候选输入已变化，请重新发送"}
	}
	inputRaw, _ := json.Marshal(input)
	outputRaw, _ := json.Marshal(RunOutput{Text: text})
	call := scope.CallID
	if call == "" {
		call = job.Hash(outputRaw)
	}
	intent := "copilot-artifact:" + scope.TurnID + ":" + kind + ":" + call
	id := job.NewID()
	hash := job.Hash(append(append([]byte{}, inputRaw...), outputRaw...))
	tag, err := tx.Exec(ctx, `INSERT INTO wb_runs(id,project_id,chapter_id,type,status,input,output,intent_id,input_hash,epoch,revision_id,cost,cost_status,finished_at) VALUES($1,$2,$3,$4,'Succeeded',$5,$6,$7,$8,$9,$10,0,'IncludedInCopilotAttempt',now()) ON CONFLICT(intent_id) DO NOTHING`, id, pid, c.ID, kind, inputRaw, outputRaw, intent, hash, input.Epoch, input.RevisionID)
	if err != nil {
		return Run{}, err
	}
	if tag.RowsAffected() == 0 {
		var oldHash string
		if err = tx.QueryRow(ctx, "SELECT id,input_hash FROM wb_runs WHERE intent_id=$1", intent).Scan(&id, &oldHash); err != nil {
			return Run{}, err
		}
		if oldHash != hash {
			return Run{}, &Conflict{Message: "工具调用标识已用于其他候选"}
		}
	} else {
		if err = eventTx(ctx, tx, id, "output", RunOutput{Text: text}); err != nil {
			return Run{}, err
		}
		if err = eventTx(ctx, tx, id, "status", map[string]string{"status": "Succeeded"}); err != nil {
			return Run{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return s.GetRun(ctx, id)
}

// ApplyCopilotBlueprint is an author action, never an agent tool.
func (s *Service) ApplyCopilotBlueprint(ctx context.Context, runID string, expectedVersion int64) (Chapter, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return Chapter{}, err
	}
	if err = s.RequireProjectRole(ctx, run.ProjectID, "editor"); err != nil {
		return Chapter{}, err
	}
	if run.Type != "planning" || run.Status != "Succeeded" || run.Stale || run.Output.Text == "" || !strings.HasPrefix(run.IntentID, "copilot-artifact:") {
		return Chapter{}, &Conflict{Message: "章纲提议来源无效"}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Chapter{}, err
	}
	defer tx.Rollback(ctx)
	var pid string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR UPDATE", run.ProjectID).Scan(&pid); err != nil {
		return Chapter{}, noRows(err)
	}
	c, err := scanChapter(tx.QueryRow(ctx, "SELECT "+chapterColumns+" FROM wb_chapters WHERE id=$1 AND NOT deleted FOR UPDATE", run.ChapterID))
	if err != nil {
		return c, err
	}
	if expectedVersion != c.BlueprintVersion {
		return c, &Conflict{Message: "章纲已变化，请重新比较候选"}
	}
	valid, err := snapshotCurrent(ctx, tx, run)
	if err != nil {
		return c, err
	}
	if !valid {
		return c, &Conflict{Message: "章纲提议所依据的作品或正文已变化"}
	}
	c, err = scanChapter(tx.QueryRow(ctx, "UPDATE wb_chapters SET blueprint=$2,blueprint_version=blueprint_version+1 WHERE id=$1 RETURNING "+chapterColumns, c.ID, run.Output.Text))
	if err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

var copilotParagraphBoundary = regexp.MustCompile(`\n\s*\n`)

func copilotParagraphDiff(before, after string) []CopilotDiffHunk {
	old, next := copilotParagraphBoundary.Split(before, -1), copilotParagraphBoundary.Split(after, -1)
	// Bound quadratic work for hostile documents; retain a truthful full-text
	// replacement hunk instead of guessing character offsets.
	if len(old)*len(next) > 250000 {
		return []CopilotDiffHunk{{ID: 0, Before: before, After: after, Changed: before != after, Accepted: before == after}}
	}
	dp := make([][]int, len(old)+1)
	for i := range dp {
		dp[i] = make([]int, len(next)+1)
	}
	for i := len(old) - 1; i >= 0; i-- {
		for j := len(next) - 1; j >= 0; j-- {
			if old[i] == next[j] {
				dp[i][j] = 1 + dp[i+1][j+1]
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	out := []CopilotDiffHunk{}
	i, j := 0, 0
	for i < len(old) || j < len(next) {
		if i < len(old) && j < len(next) && old[i] == next[j] {
			out = append(out, CopilotDiffHunk{ID: len(out), Before: old[i], After: next[j], Accepted: true})
			i++
			j++
			continue
		}
		removed, added := []string{}, []string{}
		for (i < len(old) || j < len(next)) && !(i < len(old) && j < len(next) && old[i] == next[j]) {
			if i < len(old) && (j == len(next) || dp[i+1][j] >= dp[i][j+1]) {
				removed = append(removed, old[i])
				i++
			} else {
				added = append(added, next[j])
				j++
			}
		}
		out = append(out, CopilotDiffHunk{ID: len(out), Before: strings.Join(removed, "\n\n"), After: strings.Join(added, "\n\n"), Changed: true})
	}
	return out
}
