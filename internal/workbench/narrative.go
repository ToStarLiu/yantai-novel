package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

type Evidence struct {
	RevisionID    string `json:"revisionId"`
	PlainTextHash string `json:"plainTextHash"`
	Quote         string `json:"quote"`
	Prefix        string `json:"prefix"`
	Suffix        string `json:"suffix"`
}
type MemoryChange struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Subject     string   `json:"subject"`
	Predicate   string   `json:"predicate"`
	Object      string   `json:"object"`
	CharacterID string   `json:"characterId"`
	TruthStatus string   `json:"truthStatus"`
	StoryTime   *int     `json:"storyTime"`
	Evidence    Evidence `json:"evidence"`
	Decision    string   `json:"decision"`
	EntityName  string   `json:"entityName,omitempty"`
}
type Fact struct {
	ID               string   `json:"id"`
	ProjectID        string   `json:"projectId"`
	ChapterID        string   `json:"chapterId"`
	ChapterOrder     int      `json:"chapterOrder"`
	SourceRevisionID string   `json:"sourceRevisionId"`
	StateID          string   `json:"stateId"`
	Kind             string   `json:"kind"`
	Subject          string   `json:"subject"`
	Predicate        string   `json:"predicate"`
	Object           string   `json:"object"`
	CharacterID      string   `json:"characterId"`
	TruthStatus      string   `json:"truthStatus"`
	StoryTime        *int     `json:"storyTime"`
	Evidence         Evidence `json:"evidence"`
}
type MemoryContext struct {
	ProjectID        string    `json:"projectId"`
	ChapterID        string    `json:"chapterId"`
	SourceRevisionID string    `json:"sourceRevisionId"`
	BaseStateID      string    `json:"baseStateId"`
	Text             string    `json:"text"`
	PlainTextHash    string    `json:"plainTextHash"`
	Epoch            int64     `json:"epoch"`
	Facts            []Fact    `json:"facts"`
	Entities         []Setting `json:"entities"`
	EntityIDs        []string  `json:"entityIds"`
}
type Proposal struct {
	ID               string         `json:"id"`
	ProjectID        string         `json:"projectId"`
	ChapterID        string         `json:"chapterId"`
	SourceRevisionID string         `json:"sourceRevisionId"`
	BaseStateID      string         `json:"baseStateId"`
	Epoch            int64          `json:"epoch"`
	Changes          []MemoryChange `json:"changes"`
	OriginalChanges  []MemoryChange `json:"originalChanges"`
	Status           string         `json:"status"`
	AppliedStateID   string         `json:"appliedStateId"`
	CreatedAt        time.Time      `json:"createdAt"`
}
type MemoryDecision struct {
	ID       string        `json:"id"`
	Decision string        `json:"decision"`
	Change   *MemoryChange `json:"change,omitempty"`
}
type StateView struct {
	ProjectID         string   `json:"projectId"`
	StateVersionID    string   `json:"stateVersionId"`
	Health            string   `json:"health"`
	Epoch             int64    `json:"epoch"`
	Facts             []Fact   `json:"facts"`
	PendingChapterIDs []string `json:"pendingChapterIds"`
}
type RebuildPlan struct {
	ProjectID   string   `json:"projectId"`
	Epoch       int64    `json:"epoch"`
	BaseStateID string   `json:"baseStateId"`
	ChapterIDs  []string `json:"chapterIds"`
}

func DecodeMemoryChanges(raw []byte) ([]MemoryChange, error) {
	if len(raw) > 2<<20 {
		return nil, fmt.Errorf("记忆提议过大")
	}
	var body struct {
		Changes *[]MemoryChange `json:"changes"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&body); err != nil {
		return nil, fmt.Errorf("记忆提议必须符合 memory-change-set-v1 格式")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF || body.Changes == nil {
		return nil, fmt.Errorf("记忆提议必须包含 changes 数组")
	}
	if len(*body.Changes) > 200 {
		return nil, fmt.Errorf("单章记忆提议超过200项")
	}
	return *body.Changes, nil
}

func (s *Service) MemoryInput(ctx context.Context, chapterID string) (MemoryContext, error) {
	ch, err := s.GetChapter(ctx, chapterID)
	if err != nil {
		return MemoryContext{}, err
	}
	if ch.FinalRevisionID == "" {
		return MemoryContext{}, fmt.Errorf("请先将本章正文定稿")
	}
	rev, err := s.GetRevision(ctx, ch.FinalRevisionID)
	if err != nil {
		return MemoryContext{}, err
	}
	p, err := s.GetProject(ctx, ch.ProjectID)
	if err != nil {
		return MemoryContext{}, err
	}
	state, err := s.State(ctx, ch.ProjectID)
	if err != nil {
		return MemoryContext{}, err
	}
	facts, err := factsAt(ctx, s.DB, ch.ProjectID, ch.Order-1)
	if err != nil {
		return MemoryContext{}, err
	}
	result := MemoryContext{ProjectID: ch.ProjectID, ChapterID: chapterID, SourceRevisionID: rev.ID, BaseStateID: state.StateVersionID, Text: rev.PlainText, PlainTextHash: rev.Hash, Epoch: p.Project.Epoch, Facts: facts, Entities: p.Settings, EntityIDs: []string{}}
	for _, entity := range p.Settings {
		result.EntityIDs = append(result.EntityIDs, entity.ID)
	}
	return result, nil
}

func (s *Service) validateChanges(ctx context.Context, projectID string, revision Revision, changes []MemoryChange) error {
	return validateChangesWith(ctx, s.DB, projectID, revision, changes)
}

func validateChangesWith(ctx context.Context, db queryer, projectID string, revision Revision, changes []MemoryChange) error {
	if len(changes) > 200 {
		return fmt.Errorf("单章记忆提议超过200项")
	}
	entities := map[string]bool{}
	entityKinds := map[string]string{}
	rows, err := db.Query(ctx, "SELECT id,kind FROM wb_settings WHERE project_id=$1", projectID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, kind string
		if err = rows.Scan(&id, &kind); err != nil {
			rows.Close()
			return err
		}
		entities[id] = true
		entityKinds[id] = kind
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	temporary := map[string]string{}
	for _, c := range changes {
		if strings.HasPrefix(c.Subject, "new:") && strings.TrimSpace(c.EntityName) != "" {
			if name, ok := temporary[c.Subject]; ok && name != c.EntityName {
				return fmt.Errorf("新实体临时引用名称冲突")
			}
			temporary[c.Subject] = c.EntityName
		}
	}
	changeKeys := map[string]bool{}
	for i, c := range changes {
		switch c.Kind {
		case "Location", "Injury", "Ownership", "Knowledge", "Foreshadow":
		default:
			return fmt.Errorf("第%d项记忆类型无效", i+1)
		}
		switch c.TruthStatus {
		case "True", "False", "Unknown":
		default:
			return fmt.Errorf("第%d项真值状态无效", i+1)
		}
		if strings.TrimSpace(c.Subject) == "" || strings.TrimSpace(c.Predicate) == "" || strings.TrimSpace(c.Object) == "" {
			return fmt.Errorf("第%d项缺少主体、属性或内容", i+1)
		}
		if len(c.Object) > 10000 || len(c.Predicate) > 200 {
			return fmt.Errorf("记忆内容过长")
		}
		key := c.Kind + "\x00" + c.Subject + "\x00" + c.Predicate + "\x00" + c.CharacterID
		if c.Decision != "Reject" && changeKeys[key] {
			return fmt.Errorf("同一提议包含相同主体属性的冲突变更，请合并或拒绝重复项")
		}
		if c.Decision != "Reject" {
			changeKeys[key] = true
		}
		if !entities[c.Subject] && temporary[c.Subject] == "" {
			return fmt.Errorf("第%d项主体不是本作品实体；新实体须使用new:临时引用并提供entityName", i+1)
		}
		if c.CharacterID != "" && !entities[c.CharacterID] && temporary[c.CharacterID] == "" {
			return fmt.Errorf("第%d项角色不属于本作品", i+1)
		}
		if c.Kind == "Knowledge" && c.CharacterID == "" {
			return fmt.Errorf("角色知识必须指定characterId")
		}
		if c.CharacterID != "" && entities[c.CharacterID] && entityKinds[c.CharacterID] != "character" {
			return fmt.Errorf("characterId必须指向人物实体")
		}
		if c.Kind != "Knowledge" && c.CharacterID != "" {
			return fmt.Errorf("客观事实不能填写角色知识字段")
		}
		if c.Kind != "Knowledge" && c.TruthStatus != "True" {
			return fmt.Errorf("客观事实必须有明确依据；推测只能记录为角色信念")
		}
		if c.Kind == "Ownership" && !entities[c.Object] && temporary[c.Object] == "" && c.Object != "Unknown" && c.Object != "此前未知" && c.Object != "无人持有" && c.Object != "已毁" {
			return fmt.Errorf("物品归属需引用已有持有人或明确未知/无人持有/已毁")
		}
		if c.Evidence.RevisionID != revision.ID || c.Evidence.PlainTextHash != revision.Hash || strings.TrimSpace(c.Evidence.Quote) == "" || !strings.Contains(revision.PlainText, c.Evidence.Quote) {
			return fmt.Errorf("第%d项证据不在指定定稿版本中", i+1)
		}
		if (c.Evidence.Prefix != "" || c.Evidence.Suffix != "") && !strings.Contains(revision.PlainText, c.Evidence.Prefix+c.Evidence.Quote+c.Evidence.Suffix) {
			return fmt.Errorf("第%d项证据前后文定位无效", i+1)
		}
		if c.Decision != "" && c.Decision != "Pending" && c.Decision != "Accept" && c.Decision != "Reject" {
			return fmt.Errorf("第%d项审核状态无效", i+1)
		}
	}
	return nil
}

func (s *Service) CreateProposal(ctx context.Context, chapterID, sourceRevisionID, baseStateID string, epoch int64, changes []MemoryChange) (Proposal, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Proposal{}, err
	}
	defer tx.Rollback(ctx)
	p, err := s.CreateProposalTx(ctx, tx, chapterID, sourceRevisionID, baseStateID, epoch, changes)
	if err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, err
	}
	return p, nil
}

// CreateProposalTx lets the Worker save the derived proposal, output reference,
// response and terminal job state atomically, without an additional connection.
func (s *Service) CreateProposalTx(ctx context.Context, tx pgx.Tx, chapterID, sourceRevisionID, baseStateID string, epoch int64, changes []MemoryChange) (Proposal, error) {
	var projectID string
	if err := tx.QueryRow(ctx, "SELECT project_id FROM wb_chapters WHERE id=$1 AND NOT deleted", chapterID).Scan(&projectID); err != nil {
		return Proposal{}, noRows(err)
	}
	var actualEpoch int64
	var actualBase, actualRevision string
	if err := tx.QueryRow(ctx, "SELECT epoch,state_version_id FROM wb_projects WHERE id=$1 FOR UPDATE", projectID).Scan(&actualEpoch, &actualBase); err != nil {
		return Proposal{}, err
	}
	if err := tx.QueryRow(ctx, "SELECT final_revision_id FROM wb_chapters WHERE id=$1 AND NOT deleted", chapterID).Scan(&actualRevision); err != nil {
		return Proposal{}, err
	}
	if actualEpoch != epoch || actualBase != baseStateID || actualRevision != sourceRevisionID || sourceRevisionID == "" {
		return Proposal{}, &Conflict{Message: "记忆基线或正文已变化，请重新提取"}
	}
	rev, err := scanRevision(tx.QueryRow(ctx, "SELECT "+revisionColumns+" FROM wb_revisions WHERE id=$1 AND chapter_id=$2", sourceRevisionID, chapterID))
	if err != nil {
		return Proposal{}, err
	}
	if err = validateChangesWith(ctx, tx, projectID, rev, changes); err != nil {
		return Proposal{}, err
	}
	normalized := make([]MemoryChange, len(changes))
	copy(normalized, changes)
	for i := range normalized {
		normalized[i].ID = job.NewID()
		normalized[i].Decision = "Pending"
	}
	data, _ := json.Marshal(normalized)
	id := job.NewID()
	return scanProposal(tx.QueryRow(ctx, "INSERT INTO wb_proposals(id,project_id,chapter_id,source_revision_id,base_state_id,epoch,changes,original_changes,status) VALUES($1,$2,$3,$4,$5,$6,$7,$7,'Pending') RETURNING id,project_id,chapter_id,source_revision_id,base_state_id,epoch,changes,original_changes,status,applied_state_id,created_at", id, projectID, chapterID, sourceRevisionID, baseStateID, epoch, data))
}

const proposalSelect = "SELECT id,project_id,chapter_id,source_revision_id,base_state_id,epoch,changes,original_changes,status,applied_state_id,created_at FROM wb_proposals"

func scanProposal(row pgx.Row) (Proposal, error) {
	var p Proposal
	var changes, original []byte
	err := row.Scan(&p.ID, &p.ProjectID, &p.ChapterID, &p.SourceRevisionID, &p.BaseStateID, &p.Epoch, &changes, &original, &p.Status, &p.AppliedStateID, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(changes, &p.Changes); err != nil {
		return p, err
	}
	err = json.Unmarshal(original, &p.OriginalChanges)
	return p, err
}
func (s *Service) GetProposal(ctx context.Context, id string) (Proposal, error) {
	return scanProposal(s.DB.QueryRow(ctx, proposalSelect+" WHERE id=$1", id))
}
func (s *Service) ListProposals(ctx context.Context, projectID string) ([]Proposal, error) {
	rows, err := s.DB.Query(ctx, proposalSelect+" WHERE project_id=$1 ORDER BY created_at DESC LIMIT 200", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Proposal{}
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
func (s *Service) DecideProposal(ctx context.Context, id string, decisions []MemoryDecision) (Proposal, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Proposal{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanProposal(tx.QueryRow(ctx, proposalSelect+" WHERE id=$1 FOR UPDATE", id))
	if err != nil {
		return p, err
	}
	if p.Status != "Pending" {
		return p, &Conflict{Message: "已提交的记忆提议不可再次修改"}
	}
	indices := map[string]int{}
	for i, c := range p.Changes {
		indices[c.ID] = i
	}
	seen := map[string]bool{}
	for _, decision := range decisions {
		i, ok := indices[decision.ID]
		if !ok || seen[decision.ID] {
			return p, fmt.Errorf("无效或重复的记忆变更ID")
		}
		seen[decision.ID] = true
		if decision.Decision != "Accept" && decision.Decision != "Reject" && decision.Decision != "Pending" {
			return p, fmt.Errorf("审核状态只能为Accept/Reject/Pending")
		}
		if decision.Change != nil {
			edited := *decision.Change
			edited.ID = decision.ID
			edited.Decision = decision.Decision
			p.Changes[i] = edited
		} else {
			p.Changes[i].Decision = decision.Decision
		}
	}
	rev, err := scanRevision(tx.QueryRow(ctx, "SELECT "+revisionColumns+" FROM wb_revisions WHERE id=$1", p.SourceRevisionID))
	if err != nil {
		return p, err
	}
	if err = validateChangesWith(ctx, tx, p.ProjectID, rev, p.Changes); err != nil {
		return p, err
	}
	data, _ := json.Marshal(p.Changes)
	if _, err = tx.Exec(ctx, "UPDATE wb_proposals SET changes=$2 WHERE id=$1", id, data); err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, err
	}
	return p, nil
}

// ApplyProposal checks version, epoch and author decisions in the same locked
// transaction as facts, snapshot and activity changes. Replays never add facts.
func (s *Service) ApplyProposal(ctx context.Context, id, expectedStateID, sourceRevisionID string) (StateView, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return StateView{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanProposal(tx.QueryRow(ctx, proposalSelect+" WHERE id=$1 FOR UPDATE", id))
	if err != nil {
		return StateView{}, err
	}
	if p.Status == "Applied" {
		if expectedStateID != p.BaseStateID || sourceRevisionID != p.SourceRevisionID {
			return StateView{}, &Conflict{Message: "提交参数与已应用的提议不符"}
		}
		return s.State(ctx, p.ProjectID)
	}
	var epoch int64
	var currentState, health string
	if err = tx.QueryRow(ctx, "SELECT epoch,state_version_id,health FROM wb_projects WHERE id=$1 FOR UPDATE", p.ProjectID).Scan(&epoch, &currentState, &health); err != nil {
		return StateView{}, err
	}
	var currentRevision string
	var order int
	if err = tx.QueryRow(ctx, "SELECT final_revision_id,sort_order FROM wb_chapters WHERE id=$1 AND project_id=$2 AND NOT deleted", p.ChapterID, p.ProjectID).Scan(&currentRevision, &order); err != nil {
		return StateView{}, err
	}
	if currentState != expectedStateID || p.BaseStateID != expectedStateID || epoch != p.Epoch || currentRevision != sourceRevisionID || p.SourceRevisionID != sourceRevisionID {
		return StateView{}, &Conflict{Message: "记忆基线、正文或作品顺序已变化，不能提交过期提议"}
	}
	if expectedStateID != "" {
		var baseOrder int
		if err = tx.QueryRow(ctx, "SELECT chapter_order FROM wb_states WHERE id=$1 AND project_id=$2 AND valid", expectedStateID, p.ProjectID).Scan(&baseOrder); err != nil || baseOrder >= order {
			return StateView{}, &Conflict{Message: "需要从修改点之前的有效检查点逐章重建"}
		}
	}
	var missing bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_chapters c WHERE c.project_id=$1 AND NOT c.deleted AND c.sort_order<$2 AND c.final_revision_id<>'' AND NOT EXISTS(SELECT 1 FROM wb_states st WHERE st.project_id=c.project_id AND st.chapter_id=c.id AND st.source_revision_id=c.final_revision_id AND st.valid))", p.ProjectID, order).Scan(&missing); err != nil {
		return StateView{}, err
	}
	if missing {
		return StateView{}, &Conflict{Message: "前面仍有未确认记忆的定稿章节，请按章顺序处理"}
	}
	revision, err := scanRevision(tx.QueryRow(ctx, "SELECT "+revisionColumns+" FROM wb_revisions WHERE id=$1", sourceRevisionID))
	if err != nil {
		return StateView{}, err
	}
	if err = validateChangesWith(ctx, tx, p.ProjectID, revision, p.Changes); err != nil {
		return StateView{}, err
	}
	for _, c := range p.Changes {
		if c.Decision != "Accept" && c.Decision != "Reject" {
			return StateView{}, fmt.Errorf("请逐项接受或拒绝记忆变更后再提交")
		}
	}
	acceptedTemporary := map[string]bool{}
	for _, c := range p.Changes {
		if c.Decision == "Accept" && strings.HasPrefix(c.Subject, "new:") && c.EntityName != "" {
			acceptedTemporary[c.Subject] = true
		}
	}
	for _, c := range p.Changes {
		if c.Decision == "Accept" {
			for _, reference := range []string{c.Subject, c.CharacterID, c.Object} {
				if strings.HasPrefix(reference, "new:") && !acceptedTemporary[reference] {
					return StateView{}, fmt.Errorf("接受的变更引用了被拒绝的新实体，请同时接受其实体来源")
				}
			}
		}
	}
	// Author-locked rules are independent authorities, so their exact subject /
	// predicate overrides cannot silently be replaced by an extracted proposal.
	locked, err := tx.Query(ctx, "SELECT name,content FROM wb_settings WHERE project_id=$1 AND kind='rule'", p.ProjectID)
	if err != nil {
		return StateView{}, err
	}
	rules := map[string]string{}
	for locked.Next() {
		var k, v string
		if err = locked.Scan(&k, &v); err != nil {
			locked.Close()
			return StateView{}, err
		}
		rules[k] = v
	}
	err = locked.Err()
	locked.Close()
	if err != nil {
		return StateView{}, err
	}
	for _, c := range p.Changes {
		if c.Decision == "Accept" {
			if value, ok := rules[c.Subject+"."+c.Predicate]; ok && value != c.Object {
				return StateView{}, &Conflict{Message: "提议与作者锁定设定冲突：" + c.Subject + "." + c.Predicate}
			}
		}
	}
	stateID := job.NewID()
	empty := []byte(`{"schemaVersion":"1","facts":[]}`)
	if _, err = tx.Exec(ctx, "INSERT INTO wb_states(id,project_id,chapter_id,chapter_order,source_revision_id,base_state_id,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7)", stateID, p.ProjectID, p.ChapterID, order, sourceRevisionID, expectedStateID, empty); err != nil {
		return StateView{}, err
	}
	tempMap := map[string]string{}
	for _, c := range p.Changes {
		if c.Decision == "Accept" && strings.HasPrefix(c.Subject, "new:") {
			if _, ok := tempMap[c.Subject]; !ok {
				stableID := job.NewID()
				kind := "world"
				if c.Kind == "Location" || c.Kind == "Injury" || c.CharacterID == c.Subject {
					kind = "character"
				}
				if _, err = tx.Exec(ctx, "INSERT INTO wb_settings(id,project_id,kind,name,content) VALUES($1,$2,$3,$4,'从已确认正文创建的实体')", stableID, p.ProjectID, kind, c.EntityName); err != nil {
					return StateView{}, err
				}
				tempMap[c.Subject] = stableID
			}
		}
	}
	for _, c := range p.Changes {
		if c.Decision != "Accept" {
			continue
		}
		subject, character, object := c.Subject, c.CharacterID, c.Object
		if stable, ok := tempMap[subject]; ok {
			subject = stable
		}
		if stable, ok := tempMap[character]; ok {
			character = stable
		}
		if stable, ok := tempMap[object]; ok {
			object = stable
		}
		evidence, _ := json.Marshal(c.Evidence)
		if _, err = tx.Exec(ctx, "INSERT INTO wb_facts(id,project_id,chapter_id,chapter_order,source_revision_id,state_id,kind,subject,predicate,object,character_id,truth_status,story_time,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)", job.NewID(), p.ProjectID, p.ChapterID, order, sourceRevisionID, stateID, c.Kind, subject, c.Predicate, object, character, c.TruthStatus, c.StoryTime, evidence); err != nil {
			return StateView{}, err
		}
	}
	facts, err := factsAt(ctx, tx, p.ProjectID, order)
	if err != nil {
		return StateView{}, err
	}
	snapshot, _ := json.Marshal(struct {
		SchemaVersion string `json:"schemaVersion"`
		Facts         []Fact `json:"facts"`
	}{"1", facts})
	if _, err = tx.Exec(ctx, "UPDATE wb_states SET snapshot=$2 WHERE id=$1", stateID, snapshot); err != nil {
		return StateView{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_proposals SET status='Applied',applied_state_id=$2 WHERE id=$1", id, stateID); err != nil {
		return StateView{}, err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_chapters SET needs_review=false WHERE id=$1", p.ChapterID); err != nil {
		return StateView{}, err
	}
	if err = indexChapterTx(ctx, tx, p.ProjectID, p.ChapterID, order, sourceRevisionID, revision.Hash, revision.PlainText); err != nil {
		return StateView{}, err
	}
	var pending bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_chapters c WHERE c.project_id=$1 AND NOT c.deleted AND c.final_revision_id<>'' AND NOT EXISTS(SELECT 1 FROM wb_states st WHERE st.project_id=c.project_id AND st.chapter_id=c.id AND st.source_revision_id=c.final_revision_id AND st.valid))", p.ProjectID).Scan(&pending); err != nil {
		return StateView{}, err
	}
	newHealth := "Ready"
	if pending {
		newHealth = "MemoryPending"
		if health == "RebuildRequired" {
			newHealth = "RebuildRequired"
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_projects SET state_version_id=$2,health=$3 WHERE id=$1", p.ProjectID, stateID, newHealth); err != nil {
		return StateView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return StateView{}, err
	}
	return s.State(ctx, p.ProjectID)
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func factsAt(ctx context.Context, db queryer, projectID string, order int) ([]Fact, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT ON (f.kind,f.subject,f.predicate,f.character_id) f.id,f.project_id,f.chapter_id,f.chapter_order,f.source_revision_id,f.state_id,f.kind,f.subject,f.predicate,f.object,f.character_id,f.truth_status,f.story_time,f.evidence FROM wb_facts f JOIN wb_states st ON st.id=f.state_id AND st.valid JOIN wb_chapters c ON c.id=f.chapter_id AND NOT c.deleted AND c.final_revision_id=f.source_revision_id WHERE f.project_id=$1 AND f.active AND f.chapter_order<=$2 ORDER BY f.kind,f.subject,f.predicate,f.character_id,f.chapter_order DESC,f.created_at DESC,f.id DESC`, projectID, order)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := []Fact{}
	for rows.Next() {
		var f Fact
		var evidence []byte
		if err = rows.Scan(&f.ID, &f.ProjectID, &f.ChapterID, &f.ChapterOrder, &f.SourceRevisionID, &f.StateID, &f.Kind, &f.Subject, &f.Predicate, &f.Object, &f.CharacterID, &f.TruthStatus, &f.StoryTime, &evidence); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(evidence, &f.Evidence); err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}
func (s *Service) State(ctx context.Context, projectID string) (StateView, error) {
	view := StateView{ProjectID: projectID, Facts: []Fact{}, PendingChapterIDs: []string{}}
	if err := s.DB.QueryRow(ctx, "SELECT state_version_id,health,epoch FROM wb_projects WHERE id=$1", projectID).Scan(&view.StateVersionID, &view.Health, &view.Epoch); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return view, ErrNotFound
		}
		return view, err
	}
	// Resolve invalid state pointers conservatively even during an older restore.
	if view.StateVersionID != "" {
		var valid bool
		if err := s.DB.QueryRow(ctx, "SELECT valid FROM wb_states WHERE id=$1 AND project_id=$2", view.StateVersionID, projectID).Scan(&valid); err != nil || !valid {
			view.StateVersionID = ""
		}
	}
	var err error
	view.Facts, err = factsAt(ctx, s.DB, projectID, 2147483647)
	if err != nil {
		return view, err
	}
	rows, err := s.DB.Query(ctx, "SELECT c.id FROM wb_chapters c WHERE c.project_id=$1 AND NOT c.deleted AND c.final_revision_id<>'' AND NOT EXISTS(SELECT 1 FROM wb_states st WHERE st.project_id=c.project_id AND st.chapter_id=c.id AND st.source_revision_id=c.final_revision_id AND st.valid) ORDER BY c.sort_order", projectID)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return view, err
		}
		view.PendingChapterIDs = append(view.PendingChapterIDs, id)
	}
	return view, rows.Err()
}
func (s *Service) StartRebuild(ctx context.Context, projectID string, expectedEpoch int64) (RebuildPlan, error) {
	state, err := s.State(ctx, projectID)
	if err != nil {
		return RebuildPlan{}, err
	}
	if state.Epoch != expectedEpoch {
		return RebuildPlan{}, &Conflict{Message: "作品已变化，请刷新重建计划"}
	}
	return RebuildPlan{ProjectID: projectID, Epoch: state.Epoch, BaseStateID: state.StateVersionID, ChapterIDs: state.PendingChapterIDs}, nil
}

func MemorySchema() string {
	return `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["changes"],
  "properties": {
    "changes": {
      "type": "array",
      "maxItems": 200,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "kind", "subject", "predicate", "object", "characterId", "truthStatus", "storyTime", "evidence", "decision", "entityName"],
        "properties": {
          "id": {"type": "string"},
          "kind": {"type": "string", "enum": ["Location", "Injury", "Ownership", "Knowledge", "Foreshadow"]},
          "subject": {"type": "string"},
          "predicate": {"type": "string"},
          "object": {"type": "string"},
          "characterId": {"type": "string"},
          "truthStatus": {"type": "string", "enum": ["True", "False", "Unknown"]},
          "storyTime": {"type": ["integer", "null"]},
          "evidence": {
            "type": "object",
            "additionalProperties": false,
            "required": ["revisionId", "plainTextHash", "quote", "prefix", "suffix"],
            "properties": {
              "revisionId": {"type": "string"},
              "plainTextHash": {"type": "string"},
              "quote": {"type": "string"},
              "prefix": {"type": "string"},
              "suffix": {"type": "string"}
            }
          },
          "decision": {"type": "string", "enum": ["Pending"]},
          "entityName": {"type": "string"}
        }
      }
    }
  }
}
`
}
