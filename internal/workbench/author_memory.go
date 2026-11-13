package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

type AuthorMemory struct {
	ID              string    `json:"id"`
	OwnerID         string    `json:"ownerId"`
	Scope           string    `json:"scope"`
	ProjectID       string    `json:"projectId"`
	Kind            string    `json:"kind"`
	Title           string    `json:"title"`
	Content         string    `json:"content"`
	Status          string    `json:"status"`
	Pinned          bool      `json:"pinned"`
	Version         int64     `json:"version"`
	SourceSessionID string    `json:"sourceSessionId"`
	SourceTurnID    string    `json:"sourceTurnId"`
	SourceQuote     string    `json:"sourceQuote"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
type AuthorMemoryInput struct {
	Scope           string `json:"scope"`
	ProjectID       string `json:"projectId"`
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	Content         string `json:"content"`
	Status          string `json:"status,omitempty"`
	Pinned          bool   `json:"pinned"`
	SourceSessionID string `json:"sourceSessionId,omitempty"`
	SourceTurnID    string `json:"sourceTurnId,omitempty"`
	SourceQuote     string `json:"sourceQuote,omitempty"`
}
type AuthorMemoryUpdate struct {
	ExpectedVersion int64   `json:"expectedVersion"`
	Scope           *string `json:"scope,omitempty"`
	ProjectID       *string `json:"projectId,omitempty"`
	Kind            *string `json:"kind,omitempty"`
	Title           *string `json:"title,omitempty"`
	Content         *string `json:"content,omitempty"`
	Status          *string `json:"status,omitempty"`
	Pinned          *bool   `json:"pinned,omitempty"`
}
type AuthorMemorySettings struct {
	Version         int64 `json:"version"`
	UseMemory       bool  `json:"useMemory"`
	SuggestMemories bool  `json:"suggestMemories"`
}
type AuthorMemorySettingsUpdate struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	UseMemory       bool  `json:"useMemory"`
	SuggestMemories bool  `json:"suggestMemories"`
}
type AuthorMemoryList struct {
	Items    []AuthorMemory       `json:"items"`
	Version  int64                `json:"version"`
	Settings AuthorMemorySettings `json:"settings"`
}
type AuthorMemorySnapshot struct {
	OwnerID string         `json:"ownerId"`
	Version int64          `json:"version"`
	Hash    string         `json:"hash"`
	Enabled bool           `json:"enabled"`
	Items   []AuthorMemory `json:"items"`
}
type AuthorMemoryExport struct {
	FormatVersion int                 `json:"formatVersion"`
	Items         []AuthorMemoryInput `json:"items"`
}
type AuthorMemoryImport struct {
	Items []AuthorMemoryInput `json:"items"`
}
type AuthorMemoryImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

const authorMemoryColumns = `id,owner_id,scope,project_id,kind,title,content,status,pinned,version,source_session_id,source_turn_id,source_quote,created_at,updated_at`

func scanAuthorMemory(row pgx.Row) (v AuthorMemory, err error) {
	err = row.Scan(&v.ID, &v.OwnerID, &v.Scope, &v.ProjectID, &v.Kind, &v.Title, &v.Content, &v.Status, &v.Pinned, &v.Version, &v.SourceSessionID, &v.SourceTurnID, &v.SourceQuote, &v.CreatedAt, &v.UpdatedAt)
	return v, noRows(err)
}
func authorMemoryFingerprint(content string) string {
	return job.Hash([]byte(strings.ToLower(strings.Join(strings.Fields(content), " "))))
}

var memoryCredential = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]{16,}|ghp_[a-z0-9]{20,}|github_pat_[a-z0-9_]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

func validateAuthorMemory(in AuthorMemoryInput) (AuthorMemoryInput, error) {
	in.Scope = strings.TrimSpace(in.Scope)
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	if in.Scope == "" {
		in.Scope = "author"
	}
	if in.Kind == "" {
		in.Kind = "preference"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Scope != "author" && in.Scope != "book" {
		return in, fmt.Errorf("记忆范围须为author或book")
	}
	if (in.Scope == "author" && in.ProjectID != "") || (in.Scope == "book" && in.ProjectID == "") {
		return in, fmt.Errorf("作者记忆不绑定作品，单书记忆须选择作品")
	}
	if in.Kind != "preference" && in.Kind != "voice" && in.Kind != "workflow" && in.Kind != "conversation" {
		return in, fmt.Errorf("记忆类别无效")
	}
	if in.Status != "active" && in.Status != "pending" && in.Status != "disabled" && in.Status != "rejected" {
		return in, fmt.Errorf("记忆状态无效")
	}
	if n := len([]rune(in.Title)); n < 1 || n > 80 {
		return in, fmt.Errorf("记忆标题需要1至80字")
	}
	if n := len([]rune(in.Content)); n < 1 || n > 1200 {
		return in, fmt.Errorf("记忆正文需要1至1200字")
	}
	if len([]rune(in.SourceQuote)) > 2000 || len(in.SourceSessionID) > 80 || len(in.SourceTurnID) > 80 {
		return in, fmt.Errorf("记忆来源过长")
	}
	for _, r := range in.Title + in.Content + in.SourceQuote {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return in, fmt.Errorf("记忆包含无效控制字符")
		}
	}
	if memoryCredential.MatchString(in.Title + in.Content + in.SourceQuote) {
		return in, fmt.Errorf("请勿将密钥或访问凭据保存为作者记忆")
	}
	if in.SourceTurnID == "" && (in.SourceSessionID != "" || in.SourceQuote != "") {
		return in, fmt.Errorf("对话来源需提供具体作者轮次")
	}
	return in, nil
}
func inputFromAuthorMemory(v AuthorMemory) AuthorMemoryInput {
	return AuthorMemoryInput{Scope: v.Scope, ProjectID: v.ProjectID, Kind: v.Kind, Title: v.Title, Content: v.Content, Status: v.Status, Pinned: v.Pinned, SourceSessionID: v.SourceSessionID, SourceTurnID: v.SourceTurnID, SourceQuote: v.SourceQuote}
}
func (s *Service) validateAuthorMemoryProject(ctx context.Context, in AuthorMemoryInput) error {
	if in.Scope == "book" {
		if err := s.RequireProjectRole(ctx, in.ProjectID, "viewer"); err != nil {
			return err
		}
		var exists bool
		if err := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_projects WHERE id=$1)", in.ProjectID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}
func validateAuthorMemoryEvidence(ctx context.Context, tx pgx.Tx, owner string, in *AuthorMemoryInput) error {
	if in.SourceTurnID == "" {
		return nil
	}
	var sessionID, pid, text string
	err := tx.QueryRow(ctx, `SELECT t.session_id,t.project_id,t.input->>'text' FROM wb_copilot_turns t JOIN wb_copilot_sessions s ON s.id=t.session_id WHERE t.id=$1 AND t.owner_id=$2 AND s.owner_id=$2`, in.SourceTurnID, owner).Scan(&sessionID, &pid, &text)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if in.SourceQuote == "" || !strings.Contains(text, in.SourceQuote) || (in.SourceSessionID != "" && in.SourceSessionID != sessionID) || (in.Scope == "book" && in.ProjectID != pid) {
		return fmt.Errorf("记忆来源须逐字引用自己的作者消息并匹配作品")
	}
	in.SourceSessionID = sessionID
	return nil
}
func lockAuthorMemorySettings(ctx context.Context, tx pgx.Tx, owner string) (AuthorMemorySettings, int64, error) {
	if _, err := tx.Exec(ctx, "INSERT INTO wb_author_memory_settings(owner_id) VALUES($1) ON CONFLICT DO NOTHING", owner); err != nil {
		return AuthorMemorySettings{}, 0, err
	}
	var v AuthorMemorySettings
	var cv int64
	err := tx.QueryRow(ctx, "SELECT version,context_version,use_memory,suggest_memories FROM wb_author_memory_settings WHERE owner_id=$1 FOR UPDATE", owner).Scan(&v.Version, &cv, &v.UseMemory, &v.SuggestMemories)
	return v, cv, err
}
func advanceAuthorMemory(ctx context.Context, tx pgx.Tx, owner string, contextChanged bool) error {
	bump := 0
	if contextChanged {
		bump = 1
	}
	_, err := tx.Exec(ctx, "UPDATE wb_author_memory_settings SET version=version+1,context_version=context_version+$2,updated_at=now() WHERE owner_id=$1", owner, bump)
	return err
}
func recordAuthorMemoryEvent(ctx context.Context, tx pgx.Tx, owner, id string, version int64, action, content string) error {
	_, err := tx.Exec(ctx, "INSERT INTO wb_author_memory_events(id,owner_id,memory_id,version,action,content_hash) VALUES($1,$2,$3,$4,$5,$6)", job.NewID(), owner, id, version, action, job.Hash([]byte(content)))
	return err
}
func (s *Service) GetAuthorMemorySettings(ctx context.Context) (AuthorMemorySettings, error) {
	v := AuthorMemorySettings{UseMemory: true, SuggestMemories: true}
	err := s.DB.QueryRow(ctx, "SELECT version,use_memory,suggest_memories FROM wb_author_memory_settings WHERE owner_id=$1", copilotOwner(ctx)).Scan(&v.Version, &v.UseMemory, &v.SuggestMemories)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return v, err
}
func (s *Service) UpdateAuthorMemorySettings(ctx context.Context, in AuthorMemorySettingsUpdate) (AuthorMemorySettings, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return AuthorMemorySettings{}, err
	}
	defer tx.Rollback(ctx)
	owner := copilotOwner(ctx)
	v, _, err := lockAuthorMemorySettings(ctx, tx, owner)
	if err != nil {
		return v, err
	}
	if v.Version != in.ExpectedVersion {
		return v, &Conflict{Message: "记忆设置已有更新，请刷新后重试"}
	}
	if v.UseMemory == in.UseMemory && v.SuggestMemories == in.SuggestMemories {
		return v, tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, "UPDATE wb_author_memory_settings SET use_memory=$2,suggest_memories=$3 WHERE owner_id=$1", owner, in.UseMemory, in.SuggestMemories)
	if err != nil {
		return v, err
	}
	if err = advanceAuthorMemory(ctx, tx, owner, v.UseMemory != in.UseMemory); err != nil {
		return v, err
	}
	v.Version++
	v.UseMemory = in.UseMemory
	v.SuggestMemories = in.SuggestMemories
	return v, tx.Commit(ctx)
}
func (s *Service) ListAuthorMemories(ctx context.Context, pid, status string) (AuthorMemoryList, error) {
	out := AuthorMemoryList{Items: []AuthorMemory{}}
	if pid != "" {
		if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
			return out, err
		}
	}
	if status != "" && status != "active" && status != "pending" && status != "disabled" && status != "rejected" {
		return out, fmt.Errorf("记忆状态筛选无效")
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.Settings = AuthorMemorySettings{UseMemory: true, SuggestMemories: true}
	err = tx.QueryRow(ctx, "SELECT version,use_memory,suggest_memories FROM wb_author_memory_settings WHERE owner_id=$1", copilotOwner(ctx)).Scan(&out.Settings.Version, &out.Settings.UseMemory, &out.Settings.SuggestMemories)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	out.Version = out.Settings.Version
	rows, err := tx.Query(ctx, "SELECT "+authorMemoryColumns+" FROM wb_author_memories WHERE owner_id=$1 AND ($2='' OR scope='author' OR project_id=$2) AND ($3='' OR status=$3) ORDER BY pinned DESC,updated_at DESC,id", copilotOwner(ctx), pid, status)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanAuthorMemory(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	rows.Close()
	return out, tx.Commit(ctx)
}
func (s *Service) GetAuthorMemory(ctx context.Context, id string) (AuthorMemory, error) {
	return scanAuthorMemory(s.DB.QueryRow(ctx, "SELECT "+authorMemoryColumns+" FROM wb_author_memories WHERE id=$1 AND owner_id=$2", id, copilotOwner(ctx)))
}
func (s *Service) SaveAuthorMemory(ctx context.Context, in AuthorMemoryInput) (AuthorMemory, error) {
	return s.saveAuthorMemory(ctx, in, false)
}
func (s *Service) saveAuthorMemory(ctx context.Context, in AuthorMemoryInput, proposal bool) (AuthorMemory, error) {
	in, err := validateAuthorMemory(in)
	if err != nil {
		return AuthorMemory{}, err
	}
	if err = s.validateAuthorMemoryProject(ctx, in); err != nil {
		return AuthorMemory{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return AuthorMemory{}, err
	}
	defer tx.Rollback(ctx)
	owner := copilotOwner(ctx)
	settings, _, err := lockAuthorMemorySettings(ctx, tx, owner)
	if err != nil {
		return AuthorMemory{}, err
	}
	if proposal && !settings.SuggestMemories {
		return AuthorMemory{}, fmt.Errorf("作者已关闭记忆建议")
	}
	v, created, err := saveAuthorMemoryTx(ctx, tx, owner, in)
	if err != nil {
		return v, err
	}
	if !proposal && !created && v.Status != in.Status {
		return v, &Conflict{Message: "已有相同内容的记忆，请到已有条目确认、启用或修改"}
	}
	return v, tx.Commit(ctx)
}
func saveAuthorMemoryTx(ctx context.Context, tx pgx.Tx, owner string, in AuthorMemoryInput) (AuthorMemory, bool, error) {
	if err := validateAuthorMemoryEvidence(ctx, tx, owner, &in); err != nil {
		return AuthorMemory{}, false, err
	}
	fingerprint := authorMemoryFingerprint(in.Content)
	existing, err := scanAuthorMemory(tx.QueryRow(ctx, "SELECT "+authorMemoryColumns+" FROM wb_author_memories WHERE owner_id=$1 AND scope=$2 AND project_id=$3 AND fingerprint=$4", owner, in.Scope, in.ProjectID, fingerprint))
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return AuthorMemory{}, false, err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM wb_author_memories WHERE owner_id=$1", owner).Scan(&count); err != nil {
		return AuthorMemory{}, false, err
	}
	if count >= 200 {
		return AuthorMemory{}, false, fmt.Errorf("最多保存200条作者记忆，请先整理或忘记旧记忆")
	}
	id := job.NewID()
	v, err := scanAuthorMemory(tx.QueryRow(ctx, "INSERT INTO wb_author_memories(id,owner_id,scope,project_id,kind,title,content,status,pinned,source_session_id,source_turn_id,source_quote,fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING "+authorMemoryColumns, id, owner, in.Scope, in.ProjectID, in.Kind, in.Title, in.Content, in.Status, in.Pinned, in.SourceSessionID, in.SourceTurnID, in.SourceQuote, fingerprint))
	if err != nil {
		return v, false, err
	}
	if err = recordAuthorMemoryEvent(ctx, tx, owner, id, 1, "create", in.Content); err != nil {
		return v, false, err
	}
	if err = advanceAuthorMemory(ctx, tx, owner, in.Status == "active"); err != nil {
		return v, false, err
	}
	return v, true, nil
}
func (s *Service) ProposeAuthorMemory(ctx context.Context, pid, turnID string, in AuthorMemoryInput) (AuthorMemory, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return AuthorMemory{}, err
	}
	in.SourceTurnID = turnID
	in.Status = "pending"
	if in.Scope == "book" {
		if in.ProjectID != "" && in.ProjectID != pid {
			return AuthorMemory{}, ErrForbidden
		}
		in.ProjectID = pid
	}
	// A proposal must originate from this book, even for an author-wide preference.
	var sourcePID string
	err := s.DB.QueryRow(ctx, "SELECT project_id FROM wb_copilot_turns WHERE id=$1 AND owner_id=$2", turnID, copilotOwner(ctx)).Scan(&sourcePID)
	if err != nil {
		return AuthorMemory{}, noRows(err)
	}
	if sourcePID != pid {
		return AuthorMemory{}, ErrForbidden
	}
	return s.saveAuthorMemory(ctx, in, true)
}
func (s *Service) UpdateAuthorMemory(ctx context.Context, id string, in AuthorMemoryUpdate) (AuthorMemory, error) {
	v, err := s.GetAuthorMemory(ctx, id)
	if err != nil {
		return v, err
	}
	next := inputFromAuthorMemory(v)
	if in.Scope != nil {
		next.Scope = *in.Scope
	}
	if in.ProjectID != nil {
		next.ProjectID = *in.ProjectID
	}
	if in.Kind != nil {
		next.Kind = *in.Kind
	}
	if in.Title != nil {
		next.Title = *in.Title
	}
	if in.Content != nil {
		next.Content = *in.Content
	}
	if in.Status != nil {
		next.Status = *in.Status
	}
	if in.Pinned != nil {
		next.Pinned = *in.Pinned
	}
	next, err = validateAuthorMemory(next)
	if err != nil {
		return v, err
	}
	// The author can always maintain their own private entry, even after the
	// source book or conversation is removed. Binding it to a different book
	// requires fresh access and source validation; immutable evidence was
	// already verified when this entry was first recorded.
	scopeChanged := next.Scope != v.Scope || next.ProjectID != v.ProjectID
	if scopeChanged {
		if err = s.validateAuthorMemoryProject(ctx, next); err != nil {
			return v, err
		}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	owner := copilotOwner(ctx)
	if _, _, err = lockAuthorMemorySettings(ctx, tx, owner); err != nil {
		return v, err
	}
	current, err := scanAuthorMemory(tx.QueryRow(ctx, "SELECT "+authorMemoryColumns+" FROM wb_author_memories WHERE id=$1 AND owner_id=$2", id, owner))
	if err != nil {
		return v, err
	}
	if current.Version != in.ExpectedVersion || v.Version != in.ExpectedVersion {
		return v, &Conflict{Message: "这条记忆已有更新，请刷新后重试"}
	}
	if scopeChanged {
		if err = validateAuthorMemoryEvidence(ctx, tx, owner, &next); err != nil {
			return v, err
		}
	}
	var duplicate bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_author_memories WHERE owner_id=$1 AND scope=$2 AND project_id=$3 AND fingerprint=$4 AND id<>$5)", owner, next.Scope, next.ProjectID, authorMemoryFingerprint(next.Content), id).Scan(&duplicate)
	if err != nil {
		return v, err
	}
	if duplicate {
		return v, &Conflict{Message: "同一范围已有相同记忆，请整理已有记录"}
	}
	changed := v.Status == "active" || next.Status == "active"
	v, err = scanAuthorMemory(tx.QueryRow(ctx, "UPDATE wb_author_memories SET scope=$3,project_id=$4,kind=$5,title=$6,content=$7,status=$8,pinned=$9,version=version+1,fingerprint=$10,updated_at=now() WHERE id=$1 AND owner_id=$2 RETURNING "+authorMemoryColumns, id, owner, next.Scope, next.ProjectID, next.Kind, next.Title, next.Content, next.Status, next.Pinned, authorMemoryFingerprint(next.Content)))
	if err != nil {
		return v, err
	}
	if err = recordAuthorMemoryEvent(ctx, tx, owner, id, v.Version, "update", next.Content); err != nil {
		return v, err
	}
	if err = advanceAuthorMemory(ctx, tx, owner, changed); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
func (s *Service) DeleteAuthorMemory(ctx context.Context, id string, expected int64) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	owner := copilotOwner(ctx)
	if _, _, err = lockAuthorMemorySettings(ctx, tx, owner); err != nil {
		return err
	}
	v, err := scanAuthorMemory(tx.QueryRow(ctx, "SELECT "+authorMemoryColumns+" FROM wb_author_memories WHERE id=$1 AND owner_id=$2", id, owner))
	if err != nil {
		return err
	}
	if expected != v.Version {
		return &Conflict{Message: "这条记忆已有更新，请刷新后再忘记"}
	}
	if _, err = tx.Exec(ctx, "DELETE FROM wb_author_memories WHERE id=$1 AND owner_id=$2", id, owner); err != nil {
		return err
	}
	if err = recordAuthorMemoryEvent(ctx, tx, owner, id, v.Version+1, "forget", v.Content); err != nil {
		return err
	}
	if err = advanceAuthorMemory(ctx, tx, owner, v.Status == "active"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func AuthorMemoryVersion(ctx context.Context, db skillRowDB, owner string) (int64, error) {
	var version int64
	// Candidate validation holds this lock through its commit; activation, edits
	// and forgetting take the exclusive settings lock before changing content.
	err := db.QueryRow(ctx, "SELECT context_version FROM wb_author_memory_settings WHERE owner_id=$1 FOR SHARE", owner).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return version, err
}
func ValidateAuthorMemorySnapshot(ctx context.Context, db skillRowDB, snapshot *AuthorMemorySnapshot) (bool, error) {
	if snapshot == nil {
		return true, nil
	}
	version, err := AuthorMemoryVersion(ctx, db, snapshot.OwnerID)
	return err == nil && version == snapshot.Version, err
}
func (s *Service) ResolveAuthorMemories(ctx context.Context, pid, query string) (AuthorMemorySnapshot, error) {
	out := AuthorMemorySnapshot{OwnerID: copilotOwner(ctx), Items: []AuthorMemory{}}
	if pid != "" {
		if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
			return out, err
		}
	}
	// Even an empty snapshot has a durable row, so later validation can lock it
	// against a first memory activation as well as subsequent edits.
	if _, err := s.DB.Exec(ctx, "INSERT INTO wb_author_memory_settings(owner_id) VALUES($1) ON CONFLICT DO NOTHING", out.OwnerID); err != nil {
		return out, err
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	settings := AuthorMemorySettings{UseMemory: true, SuggestMemories: true}
	err = tx.QueryRow(ctx, "SELECT context_version,use_memory FROM wb_author_memory_settings WHERE owner_id=$1", out.OwnerID).Scan(&out.Version, &settings.UseMemory)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return out, err
	}
	out.Enabled = settings.UseMemory
	all := []AuthorMemory{}
	if out.Enabled {
		rows, err := tx.Query(ctx, "SELECT "+authorMemoryColumns+" FROM wb_author_memories WHERE owner_id=$1 AND status='active' AND (scope='author' OR project_id=$2) ORDER BY pinned DESC,updated_at DESC,id", out.OwnerID, pid)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			v, err := scanAuthorMemory(rows)
			if err != nil {
				rows.Close()
				return out, err
			}
			all = append(all, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	// Relevant book preferences outrank author defaults; conversation bookmarks
	// require a query match unless explicitly pinned. Never truncate a saved rule.
	score := func(v AuthorMemory) int {
		n := 0
		if v.Scope == "book" {
			n += 10
		}
		if v.Pinned {
			n += 30
		}
		hay := strings.ToLower(v.Title + " " + v.Content)
		matched := false
		for _, token := range authorMemoryQueryTokens(query) {
			if strings.Contains(hay, token) {
				matched = true
				n += 2
			}
		}
		if v.Kind == "conversation" && !v.Pinned && !matched {
			return -1
		}
		return n
	}
	sort.SliceStable(all, func(i, j int) bool { return score(all[i]) > score(all[j]) })
	chars := 0
	for _, v := range all {
		if score(v) < 0 {
			continue
		}
		n := len([]rune(v.Title)) + len([]rune(v.Content))
		if chars+n > 1600 {
			continue
		}
		out.Items = append(out.Items, v)
		chars += n
		if len(out.Items) == 6 {
			break
		}
	}
	raw, _ := json.Marshal(struct {
		Version int64
		Enabled bool
		Items   []AuthorMemory
	}{out.Version, out.Enabled, out.Items})
	out.Hash = job.Hash(raw)
	return out, tx.Commit(ctx)
}

// Han bigrams keep short Chinese requests searchable without an external
// tokenizer. Latin words remain intact to avoid matching arbitrary fragments.
func authorMemoryQueryTokens(query string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		if len([]rune(s)) > 1 && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	var word []rune
	var previous rune
	flush := func() { add(string(word)); word = nil }
	for _, r := range []rune(strings.ToLower(query)) {
		if unicode.Is(unicode.Han, r) {
			flush()
			if previous != 0 {
				add(string([]rune{previous, r}))
			}
			previous = r
		} else {
			previous = 0
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				word = append(word, r)
			} else {
				flush()
			}
		}
		if len(out) >= 128 {
			break
		}
	}
	flush()
	return out
}
func (s *Service) ExportAuthorMemories(ctx context.Context) (AuthorMemoryExport, error) {
	list, err := s.ListAuthorMemories(ctx, "", "")
	out := AuthorMemoryExport{FormatVersion: 1, Items: []AuthorMemoryInput{}}
	if err != nil {
		return out, err
	}
	for _, v := range list.Items {
		item := inputFromAuthorMemory(v)
		item.SourceSessionID = ""
		item.SourceTurnID = ""
		item.SourceQuote = ""
		out.Items = append(out.Items, item)
	}
	return out, nil
}
func (s *Service) ImportAuthorMemories(ctx context.Context, in AuthorMemoryImport) (AuthorMemoryImportResult, error) {
	out := AuthorMemoryImportResult{}
	if len(in.Items) > 200 {
		return out, fmt.Errorf("导入最多200条记忆")
	}
	items := []AuthorMemoryInput{}
	for _, item := range in.Items {
		if item.SourceSessionID != "" || item.SourceTurnID != "" || item.SourceQuote != "" {
			return out, fmt.Errorf("导入不接受伪造的对话来源；请从原始对话重新记住")
		}
		var err error
		item, err = validateAuthorMemory(item)
		if err != nil {
			return out, err
		}
		if err = s.validateAuthorMemoryProject(ctx, item); err != nil {
			return out, err
		}
		items = append(items, item)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	owner := copilotOwner(ctx)
	if _, _, err = lockAuthorMemorySettings(ctx, tx, owner); err != nil {
		return out, err
	}
	for _, item := range items {
		_, created, err := saveAuthorMemoryTx(ctx, tx, owner, item)
		if err != nil {
			return AuthorMemoryImportResult{}, err
		}
		if created {
			out.Imported++
		} else {
			out.Skipped++
		}
	}
	return out, tx.Commit(ctx)
}
