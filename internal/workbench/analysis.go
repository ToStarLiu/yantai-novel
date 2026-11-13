package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"novel/internal/review"
)

type StyleProfile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

func StyleTemplates() []StyleProfile {
	return []StyleProfile{
		{"dialogue", "对话与潜台词", "让人物通过选择和回应表达立场。", "让每个角色使用符合身份、关系与经历的表达；用停顿、避答和具体动作表现潜台词。避免把作者全知真相直接塞进角色对白；不得补造角色获知秘密的经历。"},
		{"action", "动作与空间", "清楚交代行动、阻力和结果。", "以明确动作和空间关系组织段落；在关键冲击处使用短句，交代谁做了什么、遇到什么阻力以及结果。保持伤势、距离、物品位置和持有关系连续，避免无依据恢复能力。"},
		{"investigation", "调查与悬念", "控制证据顺序，保留合理未知。", "只展示当前视角可观察到的线索；将事实、猜测与误信分开。通过异常细节、声音和光线制造悬念，延迟解释但不隐瞒视角人物已经明确掌握的必要事实；不提前兑现章纲要求保留的谜底。"},
		{"transition", "过渡与余波", "让事件留下结果，为下一场景交接。", "围绕上一事件的代价、人物选择和下一步目标写过渡；用少量具体细节承接位置、伤势、物品和未完成行动，保持叙事节奏，避免空泛总结和重复介绍设定。"},
		{"climax", "高潮与兑现", "通过因果和代价完成冲突。", "让高潮由此前铺垫的能力、证据和选择推动；清楚写出失败风险、行动代价和直接结果。兑现已铺设的承诺，保留尚未获准揭示的信息；不能靠临时新能力或角色突然全知解决冲突。"},
	}
}

// DeterministicReview only reports mechanically checkable signals. It does
// not assign literary scores or infer semantic contradictions from keywords.
func (s *Service) DeterministicReview(ctx context.Context, chapterID, revisionID string) ([]review.Issue, error) {
	chapter, err := s.GetChapter(ctx, chapterID)
	if err != nil {
		return nil, err
	}
	if revisionID == "" {
		revisionID = chapter.CurrentRevisionID
	}
	revision, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return nil, err
	}
	if revision.ChapterID != chapterID {
		return nil, ErrNotFound
	}
	out := []review.Issue{}
	text := revision.PlainText
	if strings.TrimSpace(text) == "" {
		return out, nil
	}
	first := firstQuote(text)
	add := func(kind, severity, quote, explanation, action string) {
		if len(out) < 50 && quote != "" && strings.Contains(text, quote) {
			out = append(out, review.Issue{Kind: kind, Severity: severity, Quote: quote, Explanation: explanation, SuggestedAction: action})
		}
	}
	if strings.TrimSpace(chapter.Blueprint) == "" {
		add("Planning", "Minor", first, "本章尚未填写章纲，起草任务没有明确的计划边界。引用为本次检查的正文位置，不代表该句本身错误。", "填写本章目标、冲突与结束位置，再决定是否起草或修订。")
	}
	paragraphs := strings.Split(text, "\n")
	seenParagraphs := map[string]int{}
	paragraphReported := map[string]bool{}
	for _, paragraph := range paragraphs {
		normalized := strings.TrimSpace(paragraph)
		if utf8.RuneCountInString(normalized) >= 20 {
			seenParagraphs[normalized]++
			if seenParagraphs[normalized] > 1 && !paragraphReported[normalized] {
				add("Repetition", "Minor", normalized, "此完整段落在同一正文版本中重复出现。重复可以是创作意图，不自动判定文学错误。", "核对是否需要保留重复段落。 ")
				paragraphReported[normalized] = true
			}
		}
		if utf8.RuneCountInString(normalized) > 500 {
			quote := normalized
			r := []rune(quote)
			if len(r) > 120 {
				quote = string(r[:120])
			}
			add("Readability", "Minor", quote, fmt.Sprintf("该段有%d个Unicode字符，超过本地检查的500字符提示线；这只是排版和阅读检查。", utf8.RuneCountInString(normalized)), "检查是否需要按行动、说话者或视角变化分段。")
		}
	}
	seenSentences := map[string]int{}
	reported := map[string]bool{}
	for _, sentence := range splitSentences(text) {
		sentence = strings.TrimSpace(sentence)
		if utf8.RuneCountInString(sentence) < 12 || utf8.RuneCountInString(sentence) > 200 {
			continue
		}
		seenSentences[sentence]++
		if seenSentences[sentence] >= 3 && !reported[sentence] && !paragraphReported[sentence] {
			add("Repetition", "Minor", sentence, fmt.Sprintf("该完整句子在同一版本中至少出现%d次；未据此判断AI风格或语义质量。", seenSentences[sentence]), "核对重复是否承担必要的叙事作用。 ")
			reported[sentence] = true
		}
	}
	detail, err := s.GetProject(ctx, chapter.ProjectID)
	if err != nil {
		return nil, err
	}
	for _, rule := range detail.Settings {
		if rule.Kind != "rule" {
			continue
		}
		var contract struct {
			ForbiddenPhrases []string `json:"forbiddenPhrases"`
			RequiredPhrases  []string `json:"requiredPhrases"`
		}
		if json.Unmarshal([]byte(rule.Content), &contract) != nil {
			continue
		}
		for _, phrase := range contract.ForbiddenPhrases {
			if phrase != "" && len(phrase) <= 1000 && strings.Contains(text, phrase) {
				add("Rule", "Major", phrase, "正文包含作者规则“"+rule.Name+"”明确列出的禁用原文短语；本检查仅做精确匹配。", "核对该表达与作者规则；如属角色引用、否定或其他合理用法，由作者决定保留。")
			}
		}
		for _, phrase := range contract.RequiredPhrases {
			if phrase != "" && len(phrase) <= 1000 && !strings.Contains(text, phrase) {
				add("Rule", "Minor", first, "作者规则“"+rule.Name+"”要求的原文短语“"+phrase+"”未在本版本出现。引用仅定位检查正文，不推断同义表达是否满足要求。", "核对是否采用了同义表达，或调整作者规则与正文。")
			}
		}
	}
	// An explicit dueChapter + status contract is actionable; arbitrary prose
	// about a foreshadowing item is not guessed to be overdue.
	facts, err := factsAt(ctx, s.DB, chapter.ProjectID, chapter.Order)
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	due := map[string]int{}
	for _, fact := range facts {
		if fact.Kind != "Foreshadow" {
			continue
		}
		if fact.Predicate == "status" {
			status[fact.Subject] = strings.ToLower(fact.Object)
		}
		if fact.Predicate == "dueChapter" {
			if n, e := strconv.Atoi(fact.Object); e == nil {
				due[fact.Subject] = n
			}
		}
	}
	subjects := []string{}
	for subject := range due {
		subjects = append(subjects, subject)
	}
	sort.Strings(subjects)
	for _, subject := range subjects {
		n := due[subject]
		if n > 0 && chapter.Order >= n && (status[subject] == "open" || status[subject] == "pending" || status[subject] == "unfulfilled") {
			add("Foreshadow", "Minor", first, fmt.Sprintf("已确认伏笔%s明确计划在第%d章核对，当前确认状态仍为%s。该提醒不推断本章正文是否已经兑现。", subject, n, status[subject]), "核对伏笔承诺；若正文已经兑现，定稿后审核相应记忆变更。")
		}
	}
	return out, nil
}
func firstQuote(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			r := []rune(line)
			if len(r) > 120 {
				return string(r[:120])
			}
			return line
		}
	}
	return ""
}
func splitSentences(text string) []string {
	out := []string{}
	start := 0
	for i, r := range text {
		if strings.ContainsRune("。！？!?\n", r) {
			out = append(out, text[start:i+utf8.RuneLen(r)])
			start = i + utf8.RuneLen(r)
		}
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}

type StoryNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type StoryEvent struct {
	ID               string   `json:"id"`
	ChapterID        string   `json:"chapterId"`
	ChapterOrder     int      `json:"chapterOrder"`
	SourceRevisionID string   `json:"sourceRevisionId"`
	Kind             string   `json:"kind"`
	Subject          string   `json:"subject"`
	Predicate        string   `json:"predicate"`
	Object           string   `json:"object"`
	CharacterID      string   `json:"characterId,omitempty"`
	TruthStatus      string   `json:"truthStatus"`
	StoryTime        *int     `json:"storyTime"`
	Evidence         Evidence `json:"evidence"`
}
type StoryRelation struct {
	ID               string   `json:"id"`
	Source           string   `json:"source"`
	Target           string   `json:"target"`
	Predicate        string   `json:"predicate"`
	Kind             string   `json:"kind"`
	ChapterOrder     int      `json:"chapterOrder"`
	SourceRevisionID string   `json:"sourceRevisionId"`
	Evidence         Evidence `json:"evidence"`
}
type StoryGraph struct {
	ProjectID string          `json:"projectId"`
	Epoch     int64           `json:"epoch"`
	Nodes     []StoryNode     `json:"nodes"`
	Timeline  []StoryEvent    `json:"timeline"`
	Relations []StoryRelation `json:"relations"`
}

// StoryMap is a projection of confirmed, valid facts. It neither calls a model
// nor infers a graph edge from a name mentioned in free prose.
func (s *Service) StoryMap(ctx context.Context, projectID string) (StoryGraph, error) {
	graph := StoryGraph{ProjectID: projectID, Nodes: []StoryNode{}, Timeline: []StoryEvent{}, Relations: []StoryRelation{}}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return graph, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, "SELECT epoch FROM wb_projects WHERE id=$1", projectID).Scan(&graph.Epoch); err != nil {
		return graph, noRows(err)
	}
	rows, err := tx.Query(ctx, "SELECT id,name,kind FROM wb_settings WHERE project_id=$1 ORDER BY kind,name,id", projectID)
	if err != nil {
		return graph, err
	}
	ids := map[string]bool{}
	for rows.Next() {
		var node StoryNode
		if err = rows.Scan(&node.ID, &node.Name, &node.Kind); err != nil {
			rows.Close()
			return graph, err
		}
		graph.Nodes = append(graph.Nodes, node)
		ids[node.ID] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return graph, err
	}
	rows, err = tx.Query(ctx, `SELECT f.id,f.chapter_id,c.sort_order,f.source_revision_id,f.kind,f.subject,f.predicate,f.object,f.character_id,f.truth_status,f.story_time,f.evidence FROM wb_facts f JOIN wb_states st ON st.id=f.state_id AND st.project_id=f.project_id AND st.valid JOIN wb_chapters c ON c.id=f.chapter_id AND c.project_id=f.project_id AND NOT c.deleted AND c.final_revision_id=f.source_revision_id WHERE f.project_id=$1 AND f.active ORDER BY c.sort_order,f.created_at,f.id`, projectID)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var e StoryEvent
		var raw []byte
		if err = rows.Scan(&e.ID, &e.ChapterID, &e.ChapterOrder, &e.SourceRevisionID, &e.Kind, &e.Subject, &e.Predicate, &e.Object, &e.CharacterID, &e.TruthStatus, &e.StoryTime, &raw); err != nil {
			rows.Close()
			return graph, err
		}
		if err = json.Unmarshal(raw, &e.Evidence); err != nil {
			rows.Close()
			return graph, err
		}
		graph.Timeline = append(graph.Timeline, e)
		// False/unknown beliefs are shown in the timeline, never asserted as world relations.
		if e.Kind != "Knowledge" && e.TruthStatus == "True" && ids[e.Subject] && ids[e.Object] {
			graph.Relations = append(graph.Relations, StoryRelation{ID: e.ID, Source: e.Subject, Target: e.Object, Predicate: e.Predicate, Kind: e.Kind, ChapterOrder: e.ChapterOrder, SourceRevisionID: e.SourceRevisionID, Evidence: e.Evidence})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return graph, err
	}
	return graph, tx.Commit(ctx)
}
