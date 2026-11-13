package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"novel/internal/job"
	"novel/internal/llm"
	"strings"
	"time"
)

type CopilotSession struct {
	ID         string          `json:"id"`
	ProjectID  string          `json:"projectId"`
	OwnerID    string          `json:"ownerId"`
	Title      string          `json:"title"`
	ChapterID  string          `json:"chapterId"`
	Scope      string          `json:"scope"`
	Messages   json.RawMessage `json:"messages"`
	ContextKey string          `json:"contextKey"`
	Version    int64           `json:"version"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}
type CopilotSessionInput struct {
	Title     string `json:"title"`
	ChapterID string `json:"chapterId"`
	Scope     string `json:"scope"`
}
type CopilotTurnRequest struct {
	IntentID               string `json:"intentId"`
	Text                   string `json:"text"`
	ChapterID              string `json:"chapterId"`
	InputRevisionID        string `json:"inputRevisionId"`
	Selection              string `json:"selection"`
	MaxSteps               int    `json:"maxSteps"`
	Mode                   string `json:"mode"`
	PointOfViewCharacterID string `json:"pointOfViewCharacterId,omitempty"`
	Scope                  string `json:"scope,omitempty"`
	IgnoreMemory           bool   `json:"ignoreMemory,omitempty"`
}
type CopilotInput struct {
	CopilotTurnRequest
	Principal        Principal             `json:"principal"`
	Epoch            int64                 `json:"epoch"`
	BlueprintVersion int64                 `json:"blueprintVersion"`
	StateVersionID   string                `json:"stateVersionId"`
	StudioVersions   map[string]int64      `json:"studioVersions"`
	SessionVersion   int64                 `json:"sessionVersion"`
	ProviderID       string                `json:"providerId"`
	Policy           ModelPolicy           `json:"priceSnapshot"`
	MaxOutputTokens  int                   `json:"maxOutputTokens"`
	ContextKey       string                `json:"contextKey"`
	BookIndexHash    string                `json:"bookIndexHash,omitempty"`
	WritingSkills    *WritingSkillSnapshot `json:"writingSkills,omitempty"`
	AuthorMemory     *AuthorMemorySnapshot `json:"authorMemory,omitempty"`
	SuggestMemories  bool                  `json:"suggestMemories"`
}
type CopilotTurn struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionId"`
	ProjectID  string          `json:"projectId"`
	OwnerID    string          `json:"ownerId"`
	IntentID   string          `json:"intentId"`
	Input      CopilotInput    `json:"input"`
	Status     string          `json:"status"`
	Text       string          `json:"text"`
	Error      string          `json:"error"`
	Artifacts  json.RawMessage `json:"artifacts"`
	Stale      bool            `json:"stale"`
	CreatedAt  time.Time       `json:"createdAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
	LeaseOwner string          `json:"-"`
}
type CopilotSessionView struct {
	Session  CopilotSession  `json:"session"`
	Messages json.RawMessage `json:"messages"`
	Turns    []CopilotTurn   `json:"turns"`
}
type CopilotAttempt struct {
	ID         string      `json:"id"`
	TurnID     string      `json:"turnId"`
	Ordinal    int         `json:"ordinal"`
	Status     string      `json:"status"`
	Policy     ModelPolicy `json:"priceSnapshot"`
	Usage      llm.Usage   `json:"usage"`
	StartedAt  time.Time   `json:"startedAt"`
	FinishedAt *time.Time  `json:"finishedAt"`
}

const copilotSessionColumns = `id,project_id,owner_id,title,chapter_id,scope,messages,context_key,version,created_at,updated_at`
const copilotTurnColumns = `id,session_id,project_id,owner_id,intent_id,input,status,text,error,artifacts,stale,created_at,finished_at,lease_owner`

func copilotOwner(ctx context.Context) string {
	if p, ok := PrincipalFromContext(ctx); ok {
		return p.ID
	}
	return "admin"
}
func scanCopilotSession(row pgx.Row) (v CopilotSession, err error) {
	err = row.Scan(&v.ID, &v.ProjectID, &v.OwnerID, &v.Title, &v.ChapterID, &v.Scope, &v.Messages, &v.ContextKey, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, noRows(err)
}
func scanCopilotTurn(row pgx.Row) (v CopilotTurn, err error) {
	var raw []byte
	err = row.Scan(&v.ID, &v.SessionID, &v.ProjectID, &v.OwnerID, &v.IntentID, &raw, &v.Status, &v.Text, &v.Error, &v.Artifacts, &v.Stale, &v.CreatedAt, &v.FinishedAt, &v.LeaseOwner)
	if err != nil {
		return v, noRows(err)
	}
	err = json.Unmarshal(raw, &v.Input)
	return
}
func (s *Service) requireCopilotSession(ctx context.Context, id string) (CopilotSession, error) {
	v, e := scanCopilotSession(s.DB.QueryRow(ctx, "SELECT "+copilotSessionColumns+" FROM wb_copilot_sessions WHERE id=$1", id))
	if e != nil {
		return v, e
	}
	if v.OwnerID != copilotOwner(ctx) {
		return v, ErrForbidden
	}
	if e = s.RequireProjectRole(ctx, v.ProjectID, "viewer"); e != nil {
		return v, e
	}
	return v, nil
}
func (s *Service) CreateCopilotSession(ctx context.Context, pid string, in CopilotSessionInput) (CopilotSession, error) {
	if e := s.RequireProjectRole(ctx, pid, "viewer"); e != nil {
		return CopilotSession{}, e
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = "写作对话"
		if in.Scope == "book" {
			in.Title = "全书规划"
		}
	}
	if in.Scope == "" {
		in.Scope = "chapter"
	}
	if in.Scope != "book" && in.Scope != "chapter" {
		return CopilotSession{}, fmt.Errorf("会话范围须为book或chapter")
	}
	if in.Scope == "book" && in.ChapterID != "" {
		return CopilotSession{}, fmt.Errorf("全书会话不绑定章节")
	}
	if len(in.Title) > 200 {
		return CopilotSession{}, fmt.Errorf("会话标题过长")
	}
	if in.ChapterID != "" {
		c, e := s.GetChapter(ctx, in.ChapterID)
		if e != nil || c.ProjectID != pid {
			return CopilotSession{}, ErrNotFound
		}
	}
	return scanCopilotSession(s.DB.QueryRow(ctx, "INSERT INTO wb_copilot_sessions(id,project_id,owner_id,title,chapter_id,scope) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+copilotSessionColumns, job.NewID(), pid, copilotOwner(ctx), in.Title, in.ChapterID, in.Scope))
}
func (s *Service) ListCopilotSessions(ctx context.Context, pid string, scopes ...string) ([]CopilotSession, error) {
	if e := s.RequireProjectRole(ctx, pid, "viewer"); e != nil {
		return nil, e
	}
	scope := ""
	if len(scopes) > 0 {
		scope = scopes[0]
	}
	if scope != "" && scope != "book" && scope != "chapter" {
		return nil, fmt.Errorf("会话范围无效")
	}
	rows, e := s.DB.Query(ctx, "SELECT "+copilotSessionColumns+" FROM wb_copilot_sessions WHERE project_id=$1 AND owner_id=$2 AND ($3='' OR scope=$3) ORDER BY updated_at DESC LIMIT 100", pid, copilotOwner(ctx), scope)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []CopilotSession{}
	for rows.Next() {
		v, e := scanCopilotSession(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) GetCopilotSession(ctx context.Context, id string) (CopilotSessionView, error) {
	v, e := s.requireCopilotSession(ctx, id)
	if e != nil {
		return CopilotSessionView{}, e
	}
	rows, e := s.DB.Query(ctx, "SELECT "+copilotTurnColumns+" FROM wb_copilot_turns WHERE session_id=$1 ORDER BY created_at,id", id)
	if e != nil {
		return CopilotSessionView{}, e
	}
	defer rows.Close()
	out := CopilotSessionView{Session: v, Messages: v.Messages, Turns: []CopilotTurn{}}
	for rows.Next() {
		t, e := scanCopilotTurn(rows)
		if e != nil {
			return out, e
		}
		out.Turns = append(out.Turns, t)
	}
	return out, rows.Err()
}
func (s *Service) GetCopilotTurn(ctx context.Context, id string) (CopilotTurn, error) {
	v, e := scanCopilotTurn(s.DB.QueryRow(ctx, "SELECT "+copilotTurnColumns+" FROM wb_copilot_turns WHERE id=$1", id))
	if e != nil {
		return v, e
	}
	_, e = s.requireCopilotSession(ctx, v.SessionID)
	return v, e
}
func (s *Service) SubmitCopilotTurn(ctx context.Context, sid string, in CopilotTurnRequest) (CopilotTurn, error) {
	session, e := s.requireCopilotSession(ctx, sid)
	if e != nil {
		return CopilotTurn{}, e
	}
	if in.IntentID == "" || len(in.IntentID) > 128 || len(in.Text) > 20000 || len(in.Selection) > 12000 {
		return CopilotTurn{}, fmt.Errorf("对话输入或意图ID无效")
	}
	if in.Mode == "" {
		in.Mode = "chat"
	}
	if in.Mode != "chat" && in.Mode != "compact" {
		return CopilotTurn{}, fmt.Errorf("模式须为chat或compact")
	}
	if in.Mode == "chat" && strings.TrimSpace(in.Text) == "" {
		return CopilotTurn{}, fmt.Errorf("请输入对话内容")
	}
	if in.MaxSteps == 0 {
		in.MaxSteps = 8
	}
	if in.MaxSteps < 1 || in.MaxSteps > 20 {
		return CopilotTurn{}, fmt.Errorf("工具执行步数须在1至20之间")
	}
	if in.Scope != "" && in.Scope != session.Scope {
		return CopilotTurn{}, fmt.Errorf("不能在既有会话中切换书籍与章节范围")
	}
	in.Scope = session.Scope
	var c Chapter
	if in.Scope == "book" {
		if in.ChapterID != "" || in.InputRevisionID != "" || in.Selection != "" || in.PointOfViewCharacterID != "" {
			return CopilotTurn{}, fmt.Errorf("全书规划不接受章节选段或人物知识视角；请使用章节会话")
		}
	} else {
		if in.ChapterID == "" {
			in.ChapterID = session.ChapterID
		}
		c, e = s.GetChapter(ctx, in.ChapterID)
		if e != nil || c.ProjectID != session.ProjectID {
			return CopilotTurn{}, ErrNotFound
		}
		if in.InputRevisionID == "" {
			in.InputRevisionID = c.CurrentRevisionID
		}
		if in.InputRevisionID != c.CurrentRevisionID {
			return CopilotTurn{}, &Conflict{Message: "正文已变化，请保存后重新发送", CurrentRevisionID: c.CurrentRevisionID}
		}
		if in.Selection != "" {
			rev, e := s.GetRevision(ctx, in.InputRevisionID)
			if e != nil {
				return CopilotTurn{}, e
			}
			if strings.Count(rev.PlainText, in.Selection) != 1 {
				return CopilotTurn{}, fmt.Errorf("选段必须在当前正文中唯一出现")
			}
		}
		if in.PointOfViewCharacterID != "" {
			if _, e := s.NewCopilotToolContext(ctx, "pending", session.ProjectID, c.ID, in.InputRevisionID, in.Selection, in.PointOfViewCharacterID); e != nil {
				return CopilotTurn{}, e
			}
		}
	}
	requestRaw, _ := json.Marshal(struct {
		SID     string
		Request CopilotTurnRequest
	}{sid, in})
	hash := job.Hash(requestRaw)
	old, e := scanCopilotTurn(s.DB.QueryRow(ctx, "SELECT "+copilotTurnColumns+" FROM wb_copilot_turns WHERE intent_id=$1", in.IntentID))
	if e == nil {
		var oldHash string
		_ = s.DB.QueryRow(ctx, "SELECT input_hash FROM wb_copilot_turns WHERE id=$1", old.ID).Scan(&oldHash)
		if old.OwnerID != copilotOwner(ctx) || old.SessionID != sid || oldHash != hash {
			return old, &Conflict{Message: "意图已存在且输入不同"}
		}
		return old, nil
	}
	if !errors.Is(e, ErrNotFound) {
		return old, e
	}
	detail, e := s.GetProject(ctx, session.ProjectID)
	if e != nil {
		return CopilotTurn{}, e
	}
	studio, e := s.StudioPromptContext(ctx, session.ProjectID)
	if e != nil {
		return CopilotTurn{}, e
	}
	_, policy, provider, e := s.ResolveTaskProvider(ctx, "copilot")
	if e != nil {
		return CopilotTurn{}, e
	}
	prefs, e := s.Preferences(ctx, session.ProjectID)
	if e != nil {
		return CopilotTurn{}, e
	}
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		p = Principal{ID: "admin", Admin: true}
	}
	skills, e := s.ResolveWritingSkills(ctx, session.ProjectID, "copilot")
	if e != nil {
		return CopilotTurn{}, e
	}
	input := CopilotInput{CopilotTurnRequest: in, Principal: p, Epoch: detail.Project.Epoch, BlueprintVersion: c.BlueprintVersion, StateVersionID: detail.Project.StateVersionID, StudioVersions: studio.Versions, SessionVersion: session.Version, ProviderID: provider, Policy: policy, MaxOutputTokens: prefs.MaxOutputTokens}
	input.WritingSkills = &skills
	memory, e := s.ResolveAuthorMemories(ctx, session.ProjectID, in.Text)
	if e != nil {
		return CopilotTurn{}, e
	}
	memorySettings, e := s.GetAuthorMemorySettings(ctx)
	if e != nil {
		return CopilotTurn{}, e
	}
	if in.IgnoreMemory {
		memory.Enabled = false
		memory.Items = []AuthorMemory{}
		memory.Hash = "ignored"
	}
	input.AuthorMemory = &memory
	input.SuggestMemories = memorySettings.SuggestMemories && !in.IgnoreMemory
	if in.Scope == "book" {
		input.BookIndexHash, e = bookIndexHash(ctx, s.DB, session.ProjectID)
		if e != nil {
			return CopilotTurn{}, e
		}
	}
	contextRaw, _ := json.Marshal(map[string]any{"scope": in.Scope, "chapterId": c.ID, "revisionId": in.InputRevisionID, "epoch": input.Epoch, "state": input.StateVersionID, "blueprint": input.BlueprintVersion, "studio": input.StudioVersions, "owner": p.ID, "pov": in.PointOfViewCharacterID, "bookIndexHash": input.BookIndexHash, "writingSkillsVersion": skills.Version, "writingSkillsHash": skills.Hash, "authorMemoryVersion": memory.Version, "authorMemoryHash": memory.Hash, "ignoreMemory": in.IgnoreMemory})
	input.ContextKey = job.Hash(contextRaw)
	raw, _ := json.Marshal(input)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return CopilotTurn{}, e
	}
	defer tx.Rollback(ctx)
	var project string
	if e = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR UPDATE", session.ProjectID).Scan(&project); e != nil {
		return CopilotTurn{}, e
	}
	var ver int64
	if e = tx.QueryRow(ctx, "SELECT version FROM wb_copilot_sessions WHERE id=$1 FOR UPDATE", sid).Scan(&ver); e != nil {
		return CopilotTurn{}, e
	}
	if ver != session.Version {
		return CopilotTurn{}, &Conflict{Message: "对话已更新，请刷新"}
	}
	var duplicateID, duplicateHash string
	if e = tx.QueryRow(ctx, "SELECT id,input_hash FROM wb_copilot_turns WHERE intent_id=$1", in.IntentID).Scan(&duplicateID, &duplicateHash); e == nil {
		if duplicateHash != hash {
			return CopilotTurn{}, &Conflict{Message: "意图已存在且输入不同"}
		}
		return s.GetCopilotTurn(ctx, duplicateID)
	} else if e != pgx.ErrNoRows {
		return CopilotTurn{}, e
	}
	current, e := copilotSnapshotCurrent(ctx, tx, CopilotTurn{ProjectID: session.ProjectID, Input: input})
	if e != nil {
		return CopilotTurn{}, e
	}
	if !current {
		return CopilotTurn{}, &Conflict{Message: "作品已变化，请重新发送"}
	}
	var active bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_copilot_turns WHERE session_id=$1 AND status IN ('Pending','Running'))", sid).Scan(&active); e != nil {
		return CopilotTurn{}, e
	}
	if active {
		return CopilotTurn{}, &Conflict{Message: "本会话已有执行中的回复"}
	}
	id := job.NewID()
	if _, e = tx.Exec(ctx, "INSERT INTO wb_copilot_turns(id,session_id,project_id,owner_id,intent_id,input_hash,input) VALUES($1,$2,$3,$4,$5,$6,$7)", id, sid, session.ProjectID, p.ID, in.IntentID, hash, raw); e != nil {
		return CopilotTurn{}, e
	}
	if e = copilotEventTx(ctx, tx, id, "status", map[string]string{"status": "Pending"}); e != nil {
		return CopilotTurn{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return CopilotTurn{}, e
	}
	return s.GetCopilotTurn(ctx, id)
}
func copilotEventTx(ctx context.Context, tx pgx.Tx, id, kind string, data any) error {
	raw, e := json.Marshal(data)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO wb_copilot_events(turn_id,kind,data) VALUES($1,$2,$3)", id, kind, raw)
	return e
}
func (s *Service) CopilotEvents(ctx context.Context, id string, after int64) ([]RunEvent, error) {
	if _, e := s.GetCopilotTurn(ctx, id); e != nil {
		return nil, e
	}
	rows, e := s.DB.Query(ctx, "SELECT id,kind,data FROM wb_copilot_events WHERE turn_id=$1 AND id>$2 ORDER BY id LIMIT 200", id, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RunEvent{}
	for rows.Next() {
		var v RunEvent
		if e = rows.Scan(&v.ID, &v.Kind, &v.Data); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) CancelCopilotTurn(ctx context.Context, id string) (CopilotTurn, error) {
	v, e := s.GetCopilotTurn(ctx, id)
	if e != nil {
		return v, e
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer tx.Rollback(ctx)
	var status string
	if e = tx.QueryRow(ctx, "SELECT status FROM wb_copilot_turns WHERE id=$1 FOR UPDATE", id).Scan(&status); e != nil {
		return v, e
	}
	if status == "Pending" {
		status = "Cancelled"
	} else if status != "Running" {
		return v, &Conflict{Message: "对话已结束"}
	}
	_, e = tx.Exec(ctx, "UPDATE wb_copilot_turns SET cancel_requested=true,status=$2,finished_at=CASE WHEN $2='Cancelled' THEN now() ELSE finished_at END WHERE id=$1 AND status IN ('Pending','Running')", id, status)
	if e == nil {
		e = copilotEventTx(ctx, tx, id, "status", map[string]any{"status": status, "cancelRequested": true})
	}
	if e != nil {
		return v, e
	}
	if e = tx.Commit(ctx); e != nil {
		return v, e
	}
	return s.GetCopilotTurn(ctx, id)
}
func (s *Service) CopilotAttempts(ctx context.Context, id string) ([]CopilotAttempt, error) {
	if _, e := s.GetCopilotTurn(ctx, id); e != nil {
		return nil, e
	}
	rows, e := s.DB.Query(ctx, "SELECT id,turn_id,ordinal,status,policy,usage,started_at,finished_at FROM wb_copilot_attempts WHERE turn_id=$1 ORDER BY ordinal", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []CopilotAttempt{}
	for rows.Next() {
		var v CopilotAttempt
		var p, u []byte
		if e = rows.Scan(&v.ID, &v.TurnID, &v.Ordinal, &v.Status, &p, &u, &v.StartedAt, &v.FinishedAt); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(p, &v.Policy); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(u, &v.Usage); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
