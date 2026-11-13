package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

type archivedState struct {
	ID               string          `json:"id"`
	ProjectID        string          `json:"projectId"`
	ChapterID        string          `json:"chapterId"`
	ChapterOrder     int             `json:"chapterOrder"`
	SourceRevisionID string          `json:"sourceRevisionId"`
	BaseStateID      string          `json:"baseStateId"`
	Snapshot         json.RawMessage `json:"snapshot"`
	Valid            bool            `json:"valid"`
	CreatedAt        time.Time       `json:"createdAt"`
}
type archivedFact struct {
	Fact
	Active bool `json:"active"`
}
type narrativeArchive struct {
	SchemaVersion  int             `json:"schemaVersion"`
	ProjectID      string          `json:"projectId"`
	CurrentStateID string          `json:"currentStateId"`
	Health         string          `json:"health"`
	States         []archivedState `json:"states"`
	Facts          []archivedFact  `json:"facts"`
	Proposals      []Proposal      `json:"proposals"`
}

func (s *Service) ExportNarrative(ctx context.Context, projectID string) (json.RawMessage, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	raw, err := s.exportNarrativeTx(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	return raw, tx.Commit(ctx)
}
func (s *Service) exportNarrativeTx(ctx context.Context, tx pgx.Tx, projectID string) (json.RawMessage, error) {
	archive := narrativeArchive{SchemaVersion: 1, ProjectID: projectID, States: []archivedState{}, Facts: []archivedFact{}, Proposals: []Proposal{}}
	if err := tx.QueryRow(ctx, "SELECT state_version_id,health FROM wb_projects WHERE id=$1", projectID).Scan(&archive.CurrentStateID, &archive.Health); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT id,project_id,chapter_id,chapter_order,source_revision_id,base_state_id,snapshot,valid,created_at FROM wb_states WHERE project_id=$1 ORDER BY created_at,id", projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var st archivedState
		if err = rows.Scan(&st.ID, &st.ProjectID, &st.ChapterID, &st.ChapterOrder, &st.SourceRevisionID, &st.BaseStateID, &st.Snapshot, &st.Valid, &st.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		archive.States = append(archive.States, st)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, "SELECT id,project_id,chapter_id,chapter_order,source_revision_id,state_id,kind,subject,predicate,object,character_id,truth_status,story_time,evidence,active FROM wb_facts WHERE project_id=$1 ORDER BY created_at,id", projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f archivedFact
		var evidence []byte
		if err = rows.Scan(&f.ID, &f.ProjectID, &f.ChapterID, &f.ChapterOrder, &f.SourceRevisionID, &f.StateID, &f.Kind, &f.Subject, &f.Predicate, &f.Object, &f.CharacterID, &f.TruthStatus, &f.StoryTime, &evidence, &f.Active); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(evidence, &f.Evidence); err != nil {
			rows.Close()
			return nil, err
		}
		archive.Facts = append(archive.Facts, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, proposalSelect+" WHERE project_id=$1 ORDER BY created_at,id", projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		archive.Proposals = append(archive.Proposals, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return json.Marshal(archive)
}
func (s *Service) ImportNarrativeTx(ctx context.Context, tx pgx.Tx, projectID string, raw json.RawMessage, idMap map[string]string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if len(raw) > 32<<20 {
		return fmt.Errorf("叙事归档超过大小上限")
	}
	var archive narrativeArchive
	if err := json.Unmarshal(raw, &archive); err != nil || archive.SchemaVersion != 1 {
		return fmt.Errorf("不支持的叙事归档格式")
	}
	if idMap[archive.ProjectID] != projectID {
		return fmt.Errorf("叙事归档不属于所导入作品")
	}
	allIDs := map[string]bool{}
	for id := range idMap {
		allIDs[id] = true
	}
	for _, st := range archive.States {
		if st.ID == "" || allIDs[st.ID] || st.ProjectID != archive.ProjectID {
			return fmt.Errorf("归档状态ID无效")
		}
		allIDs[st.ID] = true
		idMap[st.ID] = job.NewID()
	}
	for _, f := range archive.Facts {
		if f.ID == "" || allIDs[f.ID] || f.ProjectID != archive.ProjectID {
			return fmt.Errorf("归档事实ID无效")
		}
		allIDs[f.ID] = true
		idMap[f.ID] = job.NewID()
	}
	for _, p := range archive.Proposals {
		if p.ID == "" || allIDs[p.ID] || p.ProjectID != archive.ProjectID {
			return fmt.Errorf("归档提议ID无效")
		}
		allIDs[p.ID] = true
		idMap[p.ID] = job.NewID()
	}
	mapped := func(id string) string {
		if v := idMap[id]; v != "" {
			return v
		}
		return id
	}
	mapEvidence := func(e Evidence) Evidence { e.RevisionID = mapped(e.RevisionID); return e }
	mapFact := func(f Fact) Fact {
		f.ID = mapped(f.ID)
		f.ProjectID = projectID
		f.ChapterID = mapped(f.ChapterID)
		f.SourceRevisionID = mapped(f.SourceRevisionID)
		f.StateID = mapped(f.StateID)
		f.Subject = mapped(f.Subject)
		f.CharacterID = mapped(f.CharacterID)
		f.Object = mapped(f.Object)
		f.Evidence = mapEvidence(f.Evidence)
		return f
	}
	for _, st := range archive.States {
		if idMap[st.ChapterID] == "" || idMap[st.SourceRevisionID] == "" || (st.BaseStateID != "" && idMap[st.BaseStateID] == "") {
			return fmt.Errorf("归档状态来源悬空")
		}
		var actualChapter string
		var order int
		var finalRevision string
		if err := tx.QueryRow(ctx, "SELECT c.id,c.sort_order,c.final_revision_id FROM wb_revisions r JOIN wb_chapters c ON c.id=r.chapter_id WHERE r.id=$1 AND c.project_id=$2", idMap[st.SourceRevisionID], projectID).Scan(&actualChapter, &order, &finalRevision); err != nil || actualChapter != idMap[st.ChapterID] {
			return fmt.Errorf("归档状态来源不属于该章节")
		}
		if st.Valid && (order != st.ChapterOrder || finalRevision != idMap[st.SourceRevisionID]) {
			return fmt.Errorf("有效状态与活动定稿不符")
		}
		var snapshot struct {
			SchemaVersion string `json:"schemaVersion"`
			Facts         []Fact `json:"facts"`
		}
		if err := json.Unmarshal(st.Snapshot, &snapshot); err != nil {
			return fmt.Errorf("状态快照格式无效")
		}
		for i, f := range snapshot.Facts {
			snapshot.Facts[i] = mapFact(f)
		}
		content, _ := json.Marshal(snapshot)
		if _, err := tx.Exec(ctx, "INSERT INTO wb_states(id,project_id,chapter_id,chapter_order,source_revision_id,base_state_id,snapshot,valid,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", idMap[st.ID], projectID, idMap[st.ChapterID], st.ChapterOrder, idMap[st.SourceRevisionID], idMap[st.BaseStateID], content, st.Valid, st.CreatedAt); err != nil {
			return err
		}
	}
	for _, original := range archive.Facts {
		f := mapFact(original.Fact)
		if idMap[original.ChapterID] == "" || idMap[original.SourceRevisionID] == "" || idMap[original.StateID] == "" {
			return fmt.Errorf("事实来源悬空")
		}
		if idMap[original.Subject] == "" || (original.CharacterID != "" && idMap[original.CharacterID] == "") {
			return fmt.Errorf("事实实体来源悬空")
		}
		var subjectExists, characterExists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_settings WHERE id=$1 AND project_id=$2)", f.Subject, projectID).Scan(&subjectExists); err != nil || !subjectExists {
			return fmt.Errorf("事实主体不属于导入作品")
		}
		if f.CharacterID != "" {
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_settings WHERE id=$1 AND project_id=$2 AND kind='character')", f.CharacterID, projectID).Scan(&characterExists); err != nil || !characterExists {
				return fmt.Errorf("事实角色不属于导入作品")
			}
		}
		var text, hash, chapterID string
		if err := tx.QueryRow(ctx, "SELECT r.plain_text,r.hash,r.chapter_id FROM wb_revisions r JOIN wb_chapters c ON c.id=r.chapter_id WHERE r.id=$1 AND c.project_id=$2", f.SourceRevisionID, projectID).Scan(&text, &hash, &chapterID); err != nil || chapterID != f.ChapterID || f.Evidence.RevisionID != f.SourceRevisionID || f.Evidence.PlainTextHash != hash || strings.TrimSpace(f.Evidence.Quote) == "" || !strings.Contains(text, f.Evidence.Quote) {
			return fmt.Errorf("归档事实证据无效")
		}
		evidence, _ := json.Marshal(f.Evidence)
		if _, err := tx.Exec(ctx, "INSERT INTO wb_facts(id,project_id,chapter_id,chapter_order,source_revision_id,state_id,kind,subject,predicate,object,character_id,truth_status,story_time,evidence,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)", f.ID, projectID, f.ChapterID, f.ChapterOrder, f.SourceRevisionID, f.StateID, f.Kind, f.Subject, f.Predicate, f.Object, f.CharacterID, f.TruthStatus, f.StoryTime, evidence, original.Active); err != nil {
			return err
		}
	}
	mapChanges := func(changes []MemoryChange) []MemoryChange {
		for i := range changes {
			changes[i].Subject = mapped(changes[i].Subject)
			changes[i].CharacterID = mapped(changes[i].CharacterID)
			changes[i].Object = mapped(changes[i].Object)
			changes[i].Evidence = mapEvidence(changes[i].Evidence)
		}
		return changes
	}
	var epoch int64
	if err := tx.QueryRow(ctx, "SELECT epoch FROM wb_projects WHERE id=$1", projectID).Scan(&epoch); err != nil {
		return err
	}
	for _, p := range archive.Proposals {
		if idMap[p.ChapterID] == "" || idMap[p.SourceRevisionID] == "" || (p.BaseStateID != "" && idMap[p.BaseStateID] == "") || (p.AppliedStateID != "" && idMap[p.AppliedStateID] == "") {
			return fmt.Errorf("归档记忆提议来源悬空")
		}
		changes, _ := json.Marshal(mapChanges(p.Changes))
		original, _ := json.Marshal(mapChanges(p.OriginalChanges))
		if _, err := tx.Exec(ctx, "INSERT INTO wb_proposals(id,project_id,chapter_id,source_revision_id,base_state_id,epoch,changes,original_changes,status,applied_state_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)", idMap[p.ID], projectID, idMap[p.ChapterID], idMap[p.SourceRevisionID], idMap[p.BaseStateID], epoch, changes, original, p.Status, idMap[p.AppliedStateID], p.CreatedAt); err != nil {
			return err
		}
	}
	current := idMap[archive.CurrentStateID]
	if archive.CurrentStateID != "" && current == "" {
		return fmt.Errorf("归档当前状态引用悬空")
	}
	if _, err := tx.Exec(ctx, "UPDATE wb_projects SET state_version_id=$2 WHERE id=$1", projectID, current); err != nil {
		return err
	}
	// Recompute readiness from restored authority instead of trusting a package
	// label. Import cannot turn a pending or invalid memory line into Ready.
	if _, err := tx.Exec(ctx, `UPDATE wb_projects p SET state_version_id=COALESCE((SELECT st.id FROM wb_states st JOIN wb_chapters c ON c.id=st.chapter_id AND c.final_revision_id=st.source_revision_id WHERE st.project_id=p.id AND st.valid AND NOT c.deleted ORDER BY st.chapter_order DESC,st.created_at DESC LIMIT 1),''),health=CASE WHEN EXISTS(SELECT 1 FROM wb_chapters c WHERE c.project_id=p.id AND NOT c.deleted AND c.needs_review) THEN 'RebuildRequired' WHEN EXISTS(SELECT 1 FROM wb_chapters c WHERE c.project_id=p.id AND NOT c.deleted AND c.final_revision_id<>'' AND NOT EXISTS(SELECT 1 FROM wb_states st WHERE st.chapter_id=c.id AND st.source_revision_id=c.final_revision_id AND st.valid)) THEN 'MemoryPending' ELSE 'Ready' END WHERE p.id=$1`, projectID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT c.id,c.sort_order,r.id,r.hash,r.plain_text FROM wb_chapters c JOIN wb_revisions r ON r.id=c.final_revision_id WHERE c.project_id=$1 AND NOT c.deleted AND NOT c.needs_review AND EXISTS(SELECT 1 FROM wb_states st WHERE st.chapter_id=c.id AND st.source_revision_id=r.id AND st.valid)", projectID)
	if err != nil {
		return err
	}
	type src struct {
		id                   string
		order                int
		revision, hash, text string
	}
	sources := []src{}
	for rows.Next() {
		var v src
		if err = rows.Scan(&v.id, &v.order, &v.revision, &v.hash, &v.text); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range sources {
		if err = indexChapterTx(ctx, tx, projectID, v.id, v.order, v.revision, v.hash, v.text); err != nil {
			return err
		}
	}
	return nil
}
