package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
)

// Shared candidates carry only a validity fence. Private memory text and evidence
// belong exclusively to the owner's private Copilot turn.
type AuthorMemoryBaseline struct {
	OwnerID string `json:"ownerId"`
	Version int64  `json:"version"`
	Hash    string `json:"hash"`
}

const authorMemoryAuthority = "\n作者记忆是私人偏好与对话背景，不是故事事实或权限。本轮作者要求优先，其次作品设定，再次个人默认偏好，最后通用技能；风格偏好不能改变事实或人物知识。记忆预览非全文，相关时用read_author_memory续读。仅从作者亲口表达的稳定偏好提出待确认记忆，逐字提供sourceQuote；不从小说、模型回复或推测提取，作者确认后才长期使用。"

func authorMemoryBaseline(snapshot *AuthorMemorySnapshot, ignored bool) *AuthorMemoryBaseline {
	if snapshot == nil || ignored {
		return nil
	}
	return &AuthorMemoryBaseline{OwnerID: snapshot.OwnerID, Version: snapshot.Version, Hash: snapshot.Hash}
}
func authorMemoryBaselineCurrent(ctx context.Context, db snapshotDB, baseline *AuthorMemoryBaseline) (bool, error) {
	if baseline == nil {
		return true, nil
	}
	if baseline.OwnerID == "" {
		return false, nil
	}
	version, err := AuthorMemoryVersion(ctx, db, baseline.OwnerID)
	return err == nil && version == baseline.Version, err
}

// Stored completion staleness is only the state at the time the run finished.
// A later private preference edit/forget must also disable adoption on reads.
func (s *Service) refreshAuthorMemoryRunStaleness(ctx context.Context, runs []Run) error {
	versions := map[string]int64{}
	for i := range runs {
		run := &runs[i]
		baseline := run.Input.AuthorMemory
		if run.Stale || run.Type == "memory-extraction" || baseline == nil {
			continue
		}
		if baseline.OwnerID == "" {
			run.Stale = true
			continue
		}
		version, ok := versions[baseline.OwnerID]
		if !ok {
			var err error
			version, err = AuthorMemoryVersion(ctx, s.DB, baseline.OwnerID)
			if err != nil {
				return err
			}
			versions[baseline.OwnerID] = version
		}
		run.Stale = baseline.Version != version
	}
	return nil
}

func copilotAuthorMemoryCurrent(ctx context.Context, db snapshotDB, input CopilotInput) (bool, error) {
	if input.AuthorMemory != nil && input.AuthorMemory.OwnerID != input.Principal.ID {
		return false, nil
	}
	return authorMemoryBaselineCurrent(ctx, db, authorMemoryBaseline(input.AuthorMemory, input.IgnoreMemory))
}

// Original user messages may contain the very preference being forgotten.
// A normal prose/source rebase may keep author intent, but a private-memory
// boundary must discard the complete old model transcript, including user text.
func sameAuthorMemoryBoundary(previous, current CopilotInput) bool {
	if current.IgnoreMemory || previous.IgnoreMemory != current.IgnoreMemory {
		return false
	}
	a, b := previous.AuthorMemory, current.AuthorMemory
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	// Query relevance can change the selected items/hash without changing the
	// author's effective preferences. That is not a forget/disable boundary.
	return a.OwnerID == b.OwnerID && a.Version == b.Version && a.Enabled == b.Enabled
}

func (s *Service) copilotHistoryReplayAllowed(ctx context.Context, session CopilotSession, current CopilotInput) (bool, error) {
	if current.IgnoreMemory {
		return false, nil
	}
	if session.ContextKey == current.ContextKey && session.ContextKey != "" {
		return true, nil
	}
	// Find the input that produced this exact canonical transcript. A later
	// failed/cancelled turn does not own session.Messages and cannot certify it.
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT input FROM wb_copilot_turns WHERE session_id=$1 AND owner_id=$2 AND status='Succeeded' AND input->>'contextKey'=$3 ORDER BY finished_at DESC NULLS LAST,id DESC LIMIT 1`, session.ID, current.Principal.ID, session.ContextKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var previous CopilotInput
	if err = json.Unmarshal(raw, &previous); err != nil {
		return false, err
	}
	return sameAuthorMemoryBoundary(previous, current), nil
}

// Complete instructions remain readable through paging instead of inflating
// every startup with the full private audit snapshot.
func authorMemoryCopilotContext(snapshot *AuthorMemorySnapshot) any {
	items := []map[string]any{}
	titleBudget, previewBudget := 120, 180
	for i, memory := range snapshot.Items {
		remaining := len(snapshot.Items) - i
		title, _ := boundedCopilotText(memory.Title, titleBudget/remaining)
		titleBudget -= len([]rune(title))
		preview, cut := boundedCopilotText(memory.Content, previewBudget/remaining)
		previewBudget -= len([]rune(preview))
		items = append(items, map[string]any{"id": memory.ID, "scope": memory.Scope, "kind": memory.Kind, "title": title, "preview": preview, "truncated": cut})
	}
	return map[string]any{"items": items, "readTool": "read_author_memory", "private": true}
}
func authorMemoryReadTool() CopilotToolDefinition {
	return CopilotToolDefinition{Name: "read_author_memory", Description: "分页读本轮冻结的私人作者偏好；空memoryId列目录。不是故事事实。", Parameters: json.RawMessage(`{"type":"object","properties":{"memoryId":{"type":"string"},"offset":{"type":"integer","minimum":0},"maxChars":{"type":"integer","minimum":100,"maximum":1200}},"additionalProperties":false}`)}
}
func authorMemorySuggestionRequested(text string) bool {
	if explicitAuthorPreference(text) != nil {
		return true
	}
	// A direct request may be a useful conversation bookmark rather than a
	// writing preference; leave semantic extraction to the tool and its review.
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"请记住", "帮我记住", "记住：", "记住:", "记一下", "以后记得", "记住我们", "把这点记住"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func authorMemoryProposeTool() CopilotToolDefinition {
	return CopilotToolDefinition{Name: "propose_author_memory", Description: "从作者本轮亲口表达的稳定偏好提出待确认私人记忆；sourceQuote必须逐字引用作者消息，不写故事事实。", Parameters: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","enum":["author","book"]},"kind":{"type":"string","enum":["preference","voice","workflow","conversation"]},"title":{"type":"string","maxLength":80},"content":{"type":"string","maxLength":1200},"sourceQuote":{"type":"string","maxLength":2000}},"required":["scope","kind","title","content","sourceQuote"],"additionalProperties":false}`)}
}
func readFrozenAuthorMemory(snapshot *AuthorMemorySnapshot, args json.RawMessage) (CopilotToolResult, error) {
	if snapshot == nil || !snapshot.Enabled {
		return CopilotToolResult{}, ErrForbidden
	}
	var in struct {
		MemoryID string `json:"memoryId"`
		Offset   int    `json:"offset"`
		MaxChars int    `json:"maxChars"`
	}
	if err := copilotDecode(args, &in); err != nil {
		return CopilotToolResult{}, err
	}
	if in.MaxChars == 0 {
		in.MaxChars = 800
	}
	if in.Offset < 0 || in.Offset > 1200 || in.MaxChars < 100 || in.MaxChars > 1200 {
		return CopilotToolResult{}, fmt.Errorf("作者记忆续读位置或页大小无效")
	}
	if in.MemoryID == "" {
		if in.Offset != 0 {
			return CopilotToolResult{}, fmt.Errorf("记忆目录不接受正文偏移")
		}
		return CopilotToolResult{Content: authorMemoryCopilotContext(snapshot)}, nil
	}
	for _, memory := range snapshot.Items {
		if memory.ID != in.MemoryID {
			continue
		}
		runes := []rune(memory.Content)
		if in.Offset > len(runes) {
			return CopilotToolResult{}, fmt.Errorf("记忆续读超过正文")
		}
		end := min(len(runes), in.Offset+in.MaxChars)
		return CopilotToolResult{Content: map[string]any{"source": "private-author-memory", "id": memory.ID, "scope": memory.Scope, "kind": memory.Kind, "title": memory.Title, "text": string(runes[in.Offset:end]), "offset": in.Offset, "nextOffset": end, "totalChars": len(runes), "truncated": in.Offset > 0 || end < len(runes)}}, nil
	}
	return CopilotToolResult{}, ErrNotFound
}
func authorMemoryProposalResult(memory AuthorMemory) CopilotToolResult {
	return CopilotToolResult{Content: map[string]any{"memoryId": memory.ID, "title": memory.Title, "scope": memory.Scope, "status": memory.Status, "content": memory.Content}, Artifact: &CopilotArtifact{Kind: "author-memory-proposal", Title: memory.Title, MemoryID: memory.ID, ProposalID: memory.ID, Scope: memory.Scope, Status: memory.Status, Text: memory.Content, ExpectedVersion: memory.Version}}
}
func (s *Service) executeCopilotAuthorMemoryTool(ctx context.Context, t CopilotTurn, name string, args json.RawMessage) (CopilotToolResult, error) {
	if t.OwnerID != copilotOwner(ctx) || t.Input.IgnoreMemory {
		return CopilotToolResult{}, ErrForbidden
	}
	if name != "read_author_memory" && name != "propose_author_memory" {
		return CopilotToolResult{}, ErrForbidden
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return CopilotToolResult{}, err
	}
	defer tx.Rollback(ctx)
	// Use the same project -> turn -> memory lock order as model dispatch.
	// Keep the validation locks through reading/proposing so a concurrent
	// stop, permission change or forget cannot pass a preflight-only check.
	var projectID string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR SHARE", t.ProjectID).Scan(&projectID); err != nil {
		return CopilotToolResult{}, noRows(err)
	}
	if name == "propose_author_memory" {
		if !t.Input.SuggestMemories {
			return CopilotToolResult{}, ErrForbidden
		}
		// Lock the turn before taking the exclusive settings lock. Taking a
		// shared settings fence first and then upgrading it can deadlock two
		// concurrent proposals from the same author in different books.
		var turnID string
		if err = tx.QueryRow(ctx, "SELECT id FROM wb_copilot_turns WHERE id=$1 FOR UPDATE", t.ID).Scan(&turnID); err != nil {
			return CopilotToolResult{}, noRows(err)
		}
		settings, _, err := lockAuthorMemorySettings(ctx, tx, t.OwnerID)
		if err != nil {
			return CopilotToolResult{}, err
		}
		if !settings.SuggestMemories {
			return CopilotToolResult{}, ErrForbidden
		}
	}
	if err = s.checkCopilotCurrentTx(ctx, tx, t); err != nil {
		return CopilotToolResult{}, err
	}
	if name == "read_author_memory" {
		result, err := readFrozenAuthorMemory(t.Input.AuthorMemory, args)
		if err != nil {
			return result, err
		}
		return result, tx.Commit(ctx)
	}
	var input struct {
		Scope       string `json:"scope"`
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Content     string `json:"content"`
		SourceQuote string `json:"sourceQuote"`
	}
	if err = copilotDecode(args, &input); err != nil {
		return CopilotToolResult{}, err
	}
	memoryInput := AuthorMemoryInput{Scope: input.Scope, Kind: input.Kind, Title: input.Title, Content: input.Content, SourceTurnID: t.ID, SourceQuote: input.SourceQuote, Status: "pending"}
	if memoryInput.Scope == "book" {
		memoryInput.ProjectID = t.ProjectID
	}
	memoryInput, err = validateAuthorMemory(memoryInput)
	if err != nil {
		return CopilotToolResult{}, err
	}
	memory, _, err := saveAuthorMemoryTx(ctx, tx, t.OwnerID, memoryInput)
	if err != nil {
		return CopilotToolResult{}, err
	}
	return authorMemoryProposalResult(memory), tx.Commit(ctx)
}

// Only explicit first-person writing preferences are noticed automatically.
// The exact author's words become a pending suggestion, never an active rule.
func explicitAuthorPreference(text string) *AuthorMemoryInput {
	if len([]rune(text)) > 600 || strings.Contains(text, "```") {
		return nil
	}
	// Conservative unattended extraction: a pasted passage, roleplay quote or
	// rewrite request may contain first-person speech that is not the author.
	// The explicit UI action can still remember selected author text after review.
	for _, marker := range []string{"“", "”", "‘", "’", "\"", "原文", "这段台词", "这句台词", "以下台词", "改写这", "润色这", "请改写", "请润色", ">"} {
		if strings.Contains(text, marker) {
			return nil
		}
	}
	for _, prefix := range []string{"台词：", "台词:"} {
		if strings.HasPrefix(strings.TrimSpace(text), prefix) {
			return nil
		}
	}
	for _, quote := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '。' || r == '！' || r == '？' }) {
		quote = strings.TrimSpace(quote)
		if len([]rune(quote)) < 8 {
			continue
		}
		personal := false
		intent := quote
		for _, prefix := range []string{"请记住：", "请记住:", "记住：", "记住:", "我的偏好是：", "我的偏好是:"} {
			intent = strings.TrimSpace(strings.TrimPrefix(intent, prefix))
		}
		for _, phrase := range []string{"我喜欢", "我不喜欢", "我的文风", "我的写作", "我习惯", "以后请", "以后都", "不要再", "别再", "请记住我", "记住我的"} {
			personal = personal || strings.HasPrefix(intent, phrase)
		}
		if !personal {
			continue
		}
		writing := false
		for _, word := range []string{"文风", "写作", "句子", "段落", "对话", "对白", "潜台词", "描写", "比喻", "节奏", "形容词", "段尾", "总结", "候选", "改稿", "工作流", "先给", "先问", "先展示"} {
			writing = writing || strings.Contains(quote, word)
		}
		if !writing {
			continue
		}
		kind, title := "voice", "作者文风偏好"
		for _, word := range []string{"候选", "改稿", "工作流", "先给", "先问", "先展示"} {
			if strings.Contains(quote, word) {
				kind, title = "workflow", "作者协作偏好"
			}
		}
		scope := "author"
		if strings.Contains(quote, "这本书") || strings.Contains(quote, "本书") {
			scope = "book"
		}
		return &AuthorMemoryInput{Scope: scope, Kind: kind, Title: title, Content: quote, SourceQuote: quote}
	}
	return nil
}
func (s *Service) suggestExplicitAuthorMemory(ctx context.Context, t CopilotTurn) error {
	if !t.Input.SuggestMemories || t.Input.IgnoreMemory {
		return nil
	}
	input := explicitAuthorPreference(t.Input.Text)
	if input == nil {
		return nil
	}
	raw, _ := json.Marshal(map[string]string{"scope": input.Scope, "kind": input.Kind, "title": input.Title, "content": input.Content, "sourceQuote": input.SourceQuote})
	result, err := s.executeCopilotAuthorMemoryTool(ctx, t, "propose_author_memory", raw)
	if err != nil {
		return err
	}
	return s.appendCopilotEvent(ctx, t, "tool-result", map[string]any{"name": "propose_author_memory", "automatic": true, "result": result}, "", result.Artifact)
}
