package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

const projectColumns = "id,title,description,genre,epoch,health,state_version_id,created_at"
const chapterColumns = "id,project_id,volume,title,sort_order,blueprint,blueprint_version,current_revision_id,final_revision_id,needs_review,deleted"
const revisionColumns = "id,chapter_id,parent_id,document,plain_text,hash,source,run_id,created_at"

// This matches the Unicode whitespace recognized by strings.TrimSpace.
const chapterTextWhitespace = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

func noRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func scanProject(row pgx.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Title, &p.Description, &p.Genre, &p.Epoch, &p.Health, &p.StateVersionID, &p.CreatedAt)
	return p, noRows(err)
}
func scanChapter(row pgx.Row) (Chapter, error) {
	var c Chapter
	err := row.Scan(&c.ID, &c.ProjectID, &c.Volume, &c.Title, &c.Order, &c.Blueprint, &c.BlueprintVersion, &c.CurrentRevisionID, &c.FinalRevisionID, &c.NeedsReview, &c.Deleted)
	return c, noRows(err)
}
func scanChapterSummary(row pgx.Row) (Chapter, error) {
	var c Chapter
	var hasText bool
	err := row.Scan(&c.ID, &c.ProjectID, &c.Volume, &c.Title, &c.Order, &c.Blueprint, &c.BlueprintVersion, &c.CurrentRevisionID, &c.FinalRevisionID, &c.NeedsReview, &c.Deleted, &hasText)
	c.HasText = &hasText
	return c, noRows(err)
}
func scanRevision(row pgx.Row) (Revision, error) {
	var r Revision
	err := row.Scan(&r.ID, &r.ChapterID, &r.ParentID, &r.Document, &r.PlainText, &r.Hash, &r.Source, &r.RunID, &r.CreatedAt)
	return r, noRows(err)
}
func validName(s string) bool { return strings.TrimSpace(s) != "" && len(s) <= 1000 }

func (s *Service) ListProjects(ctx context.Context) ([]Project, error) {
	query := "SELECT " + projectColumns + " FROM wb_projects"
	args := []any{}
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin {
		query += " WHERE id IN (SELECT project_id FROM wb_project_members WHERE user_id=$1)"
		args = append(args, p.ID)
	}
	rows, err := s.DB.Query(ctx, query+" ORDER BY created_at DESC,id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, e := scanProject(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) CreateProject(ctx context.Context, title, description, genre string) (Project, error) {
	if !validName(title) || len(description) > 200000 || len(genre) > 1000 {
		return Project{}, fmt.Errorf("作品名称或简介无效")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanProject(tx.QueryRow(ctx, "INSERT INTO wb_projects(id,title,description,genre) VALUES($1,$2,$3,$4) RETURNING "+projectColumns, job.NewID(), strings.TrimSpace(title), description, genre))
	if err != nil {
		return p, err
	}
	if err = ownerForNewProjectTx(ctx, tx, p.ID); err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}
func (s *Service) UpdateProject(ctx context.Context, id, title, description, genre string) (Project, error) {
	if !validName(title) || len(description) > 200000 || len(genre) > 1000 {
		return Project{}, fmt.Errorf("作品名称或简介无效")
	}
	return scanProject(s.DB.QueryRow(ctx, "UPDATE wb_projects SET title=$2,description=$3,genre=$4 WHERE id=$1 RETURNING "+projectColumns, id, strings.TrimSpace(title), description, genre))
}
func (s *Service) DeleteProject(ctx context.Context, id string) error {
	n, err := s.DB.Exec(ctx, "DELETE FROM wb_projects WHERE id=$1", id)
	if err == nil && n.RowsAffected() != 1 {
		return ErrNotFound
	}
	return err
}
func (s *Service) GetProject(ctx context.Context, id string) (ProjectDetail, error) {
	out := ProjectDetail{Chapters: []Chapter{}, Settings: []Setting{}}
	var err error
	out.Project, err = scanProject(s.DB.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1", id))
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, "SELECT "+chapterColumns+", EXISTS(SELECT 1 FROM wb_revisions current_revision WHERE current_revision.id=wb_chapters.current_revision_id AND current_revision.chapter_id=wb_chapters.id AND btrim(current_revision.plain_text,$2)<>'') FROM wb_chapters WHERE project_id=$1 AND NOT deleted ORDER BY sort_order,id", id, chapterTextWhitespace)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		c, e := scanChapterSummary(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Chapters = append(out.Chapters, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = s.DB.Query(ctx, "SELECT id,project_id,kind,name,content FROM wb_settings WHERE project_id=$1 ORDER BY kind,name,id", id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v Setting
		if err = rows.Scan(&v.ID, &v.ProjectID, &v.Kind, &v.Name, &v.Content); err != nil {
			return out, err
		}
		out.Settings = append(out.Settings, v)
	}
	return out, rows.Err()
}
func (s *Service) SaveSetting(ctx context.Context, projectID string, in Setting) (Setting, error) {
	if !validName(in.Name) || len(in.Content) > 200000 {
		return Setting{}, fmt.Errorf("设定名称或内容无效")
	}
	switch in.Kind {
	case "character", "world", "rule", "style", "outline":
	default:
		return Setting{}, fmt.Errorf("无效的设定类型")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Setting{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID)); err != nil {
		return Setting{}, err
	}
	if in.ID == "" {
		in.ID = job.NewID()
		_, err = tx.Exec(ctx, "INSERT INTO wb_settings(id,project_id,kind,name,content) VALUES($1,$2,$3,$4,$5)", in.ID, projectID, in.Kind, in.Name, in.Content)
	} else {
		var n int64
		tag, e := tx.Exec(ctx, "UPDATE wb_settings SET kind=$3,name=$4,content=$5 WHERE id=$1 AND project_id=$2", in.ID, projectID, in.Kind, in.Name, in.Content)
		err = e
		n = tag.RowsAffected()
		if err == nil && n != 1 {
			err = ErrNotFound
		}
	}
	if err != nil {
		return Setting{}, err
	}
	in.ProjectID = projectID
	if err = s.settingImpactTx(ctx, tx, projectID); err != nil {
		return Setting{}, err
	}
	return in, tx.Commit(ctx)
}
func (s *Service) DeleteSetting(ctx context.Context, projectID, id string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID)); err != nil {
		return err
	}
	var referenced bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_facts WHERE project_id=$1 AND (subject=$2 OR character_id=$2 OR object=$2))", projectID, id).Scan(&referenced); err != nil {
		return err
	}
	if referenced {
		return &Conflict{Message: "该设定已有正文记忆来源，请编辑设定内容以保留历史证据"}
	}
	n, err := tx.Exec(ctx, "DELETE FROM wb_settings WHERE id=$1 AND project_id=$2", id, projectID)
	if err != nil {
		return err
	}
	if n.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err = s.settingImpactTx(ctx, tx, projectID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) settingImpactTx(ctx context.Context, tx pgx.Tx, projectID string) error {
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM wb_chapters WHERE project_id=$1 AND final_revision_id<>'' AND NOT deleted", projectID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		_, err := s.invalidateTx(ctx, tx, projectID, 0, "作者设定变更，需核对已有状态")
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE wb_projects SET epoch=epoch+1 WHERE id=$1", projectID)
	return err
}
func (s *Service) GetChapter(ctx context.Context, id string) (Chapter, error) {
	c, err := scanChapter(s.DB.QueryRow(ctx, "SELECT "+chapterColumns+" FROM wb_chapters WHERE id=$1 AND NOT deleted", id))
	if err != nil {
		return c, err
	}
	if c.CurrentRevisionID != "" {
		r, e := s.GetRevision(ctx, c.CurrentRevisionID)
		if e != nil {
			return c, e
		}
		c.Revision = &r
	}
	hasText := c.Revision != nil && strings.TrimSpace(c.Revision.PlainText) != ""
	c.HasText = &hasText
	return c, nil
}
func (s *Service) CreateChapter(ctx context.Context, projectID, title, volume string) (Chapter, error) {
	if !validName(title) || len(volume) > 1000 {
		return Chapter{}, fmt.Errorf("章节名称无效")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Chapter{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID)); err != nil {
		return Chapter{}, err
	}
	var order int
	if err = tx.QueryRow(ctx, "SELECT COALESCE(max(sort_order),0)+1 FROM wb_chapters WHERE project_id=$1", projectID).Scan(&order); err != nil {
		return Chapter{}, err
	}
	c, err := scanChapter(tx.QueryRow(ctx, "INSERT INTO wb_chapters(id,project_id,volume,title,sort_order) VALUES($1,$2,$3,$4,$5) RETURNING "+chapterColumns, job.NewID(), projectID, volume, title, order))
	if err != nil {
		return c, err
	}
	r, err := saveRevisionTx(ctx, tx, c, SaveInput{Document: DocumentFromText(""), Source: "author"})
	if err != nil {
		return c, err
	}
	c.CurrentRevisionID = r.ID
	c.Revision = &r
	hasText := false
	c.HasText = &hasText
	if _, err = tx.Exec(ctx, "UPDATE wb_projects SET epoch=epoch+1 WHERE id=$1", projectID); err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}
func (s *Service) UpdateChapter(ctx context.Context, id, title, volume, blueprint string) (Chapter, error) {
	if !validName(title) || len(volume) > 1000 || len(blueprint) > 200000 {
		return Chapter{}, fmt.Errorf("章节标题或章纲无效")
	}
	return scanChapter(s.DB.QueryRow(ctx, "UPDATE wb_chapters SET title=$2,volume=$3,blueprint_version=blueprint_version+CASE WHEN blueprint<>$4 THEN 1 ELSE 0 END,blueprint=$4 WHERE id=$1 AND NOT deleted RETURNING "+chapterColumns, id, title, volume, blueprint))
}
func (s *Service) GetRevision(ctx context.Context, id string) (Revision, error) {
	return scanRevision(s.DB.QueryRow(ctx, "SELECT "+revisionColumns+" FROM wb_revisions WHERE id=$1", id))
}
func (s *Service) ListRevisions(ctx context.Context, chapterID string) ([]Revision, error) {
	var exists bool
	if err := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_chapters WHERE id=$1)", chapterID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := s.DB.Query(ctx, "SELECT "+revisionColumns+" FROM wb_revisions WHERE chapter_id=$1 ORDER BY created_at DESC,id DESC", chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Revision{}
	for rows.Next() {
		r, e := scanRevision(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) SaveRevision(ctx context.Context, chapterID string, in SaveInput) (Revision, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Revision{}, err
	}
	defer tx.Rollback(ctx)
	// Keep the project-before-chapter lock order used by finalization and memory
	// commits so AI source validation stays stable until this revision commits.
	var projectID string
	if err = tx.QueryRow(ctx, "SELECT project_id FROM wb_chapters WHERE id=$1 AND NOT deleted", chapterID).Scan(&projectID); err != nil {
		return Revision{}, noRows(err)
	}
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID)); err != nil {
		return Revision{}, err
	}
	c, err := scanChapter(tx.QueryRow(ctx, "SELECT "+chapterColumns+" FROM wb_chapters WHERE id=$1 AND NOT deleted FOR UPDATE", chapterID))
	if err != nil {
		return Revision{}, err
	}
	r, err := saveRevisionTx(ctx, tx, c, in)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func saveRevisionTx(ctx context.Context, tx pgx.Tx, c Chapter, in SaveInput) (Revision, error) {
	if c.CurrentRevisionID != in.ExpectedCurrentRevisionID {
		return Revision{}, &Conflict{Message: "正文已被修改，请比较当前版本", CurrentRevisionID: c.CurrentRevisionID}
	}
	document, plain, hash, err := NormalizeDocument(in.Document)
	if err != nil {
		return Revision{}, err
	}
	source := in.Source
	if source == "" {
		source = "author"
	}
	switch source {
	case "author", "ai", "restored", "import":
	default:
		return Revision{}, fmt.Errorf("无效版本来源")
	}
	if source == "ai" {
		var runChapter, runRevision, status, runType, blueprintVersion, runState, actualState string
		var studioRaw json.RawMessage
		var writingSkillsRaw json.RawMessage
		var authorMemoryRaw json.RawMessage
		var canvasIDsRaw json.RawMessage
		var canvasVersion int64
		var runEpoch, actualEpoch int64
		var stale bool
		if err = tx.QueryRow(ctx, "SELECT chapter_id,revision_id,status,stale,type,COALESCE(input->>'blueprintVersion',''),COALESCE(input->>'stateVersionId',''),epoch,COALESCE(input->'studioVersions','null'::jsonb),COALESCE(input->'canvasNodeIds','null'::jsonb),COALESCE((input->>'canvasBoardVersion')::bigint,0),COALESCE(input->'writingSkills','null'::jsonb) FROM wb_runs WHERE id=$1", in.RunID).Scan(&runChapter, &runRevision, &status, &stale, &runType, &blueprintVersion, &runState, &runEpoch, &studioRaw, &canvasIDsRaw, &canvasVersion, &writingSkillsRaw); err != nil || runChapter != c.ID || runRevision != c.CurrentRevisionID || status != "Succeeded" || stale || (runType != "draft" && runType != "revision" && runType != "rewrite") {
			return Revision{}, &Conflict{Message: "候选来源无效或输入正文已经变化，请重新比较生成结果", CurrentRevisionID: c.CurrentRevisionID}
		}
		if err = tx.QueryRow(ctx, "SELECT epoch,state_version_id FROM wb_projects WHERE id=$1", c.ProjectID).Scan(&actualEpoch, &actualState); err != nil {
			return Revision{}, err
		}
		if blueprintVersion != strconv.FormatInt(c.BlueprintVersion, 10) || runState != actualState || runEpoch != actualEpoch {
			return Revision{}, &Conflict{Message: "章纲、作品或记忆基线已经变化，请重新比较生成结果", CurrentRevisionID: c.CurrentRevisionID}
		}
		if err = tx.QueryRow(ctx, "SELECT COALESCE(input->'authorMemory','null'::jsonb) FROM wb_runs WHERE id=$1", in.RunID).Scan(&authorMemoryRaw); err != nil {
			return Revision{}, err
		}
		var privateBaseline *AuthorMemoryBaseline
		if json.Unmarshal(authorMemoryRaw, &privateBaseline) != nil {
			return Revision{}, &Conflict{Message: "候选作者记忆来源无效", CurrentRevisionID: c.CurrentRevisionID}
		}
		validMemory, memoryErr := authorMemoryBaselineCurrent(ctx, tx, privateBaseline)
		if memoryErr != nil {
			return Revision{}, memoryErr
		}
		if !validMemory {
			return Revision{}, &Conflict{Message: "作者偏好记忆已变化，请重新比较生成结果", CurrentRevisionID: c.CurrentRevisionID}
		}
		var frozenSkills *WritingSkillSnapshot
		if json.Unmarshal(writingSkillsRaw, &frozenSkills) != nil {
			return Revision{}, &Conflict{Message: "候选写作技能来源无效", CurrentRevisionID: c.CurrentRevisionID}
		}
		validSkills, e := writingSkillSnapshotCurrent(ctx, tx, c.ProjectID, frozenSkills)
		if e != nil {
			return Revision{}, e
		}
		if !validSkills {
			return Revision{}, &Conflict{Message: "写作技能已变化，请重新比较生成结果", CurrentRevisionID: c.CurrentRevisionID}
		}
		var versions map[string]int64
		if json.Unmarshal(studioRaw, &versions) != nil {
			return Revision{}, &Conflict{Message: "候选工作室来源版本无效", CurrentRevisionID: c.CurrentRevisionID}
		}
		if versions != nil {
			valid, e := ValidateStudioPromptTx(ctx, tx, c.ProjectID, versions)
			if e != nil {
				return Revision{}, e
			}
			if !valid {
				return Revision{}, &Conflict{Message: "全书规划、场景或人物弧已经变化，请重新生成或手动编辑", CurrentRevisionID: c.CurrentRevisionID}
			}
		}
		var canvasIDs []string
		if json.Unmarshal(canvasIDsRaw, &canvasIDs) != nil {
			return Revision{}, &Conflict{Message: "候选画布来源无效", CurrentRevisionID: c.CurrentRevisionID}
		}
		if len(canvasIDs) > 0 {
			valid, e := ValidateCanvasSnapshotTx(ctx, tx, c.ProjectID, canvasVersion)
			if e != nil {
				return Revision{}, e
			}
			if !valid {
				return Revision{}, &Conflict{Message: "画布规划已经变化，请重新比较候选", CurrentRevisionID: c.CurrentRevisionID}
			}
		}
	} else {
		in.RunID = ""
	}
	r, err := scanRevision(tx.QueryRow(ctx, "INSERT INTO wb_revisions(id,chapter_id,parent_id,document,plain_text,hash,source,run_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING "+revisionColumns, job.NewID(), c.ID, c.CurrentRevisionID, document, plain, hash, source, in.RunID))
	if err != nil {
		return r, err
	}
	_, err = tx.Exec(ctx, "UPDATE wb_chapters SET current_revision_id=$2 WHERE id=$1", c.ID, r.ID)
	return r, err
}
func (s *Service) RestoreRevision(ctx context.Context, chapterID, revisionID, expectedCurrentID string) (Revision, error) {
	r, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return r, err
	}
	if r.ChapterID != chapterID {
		return Revision{}, ErrNotFound
	}
	return s.SaveRevision(ctx, chapterID, SaveInput{ExpectedCurrentRevisionID: expectedCurrentID, Document: r.Document, Source: "restored"})
}
func (s *Service) PreviewImpact(ctx context.Context, chapterID string) (Impact, error) {
	c, err := s.GetChapter(ctx, chapterID)
	if err != nil {
		return Impact{}, err
	}
	out := Impact{ProjectID: c.ProjectID, EarliestOrder: c.Order, Reason: "修改已定稿正文将使本章及后续记忆失效", ChapterIDs: []string{}}
	rows, err := s.DB.Query(ctx, "SELECT id FROM wb_chapters WHERE project_id=$1 AND sort_order >=$2 AND NOT deleted ORDER BY sort_order", c.ProjectID, c.Order)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return out, err
		}
		out.ChapterIDs = append(out.ChapterIDs, id)
	}
	return out, rows.Err()
}
func (s *Service) Finalize(ctx context.Context, chapterID, revisionID string) (Impact, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Impact{}, err
	}
	defer tx.Rollback(ctx)
	var projectID string
	if err = tx.QueryRow(ctx, "SELECT project_id FROM wb_chapters WHERE id=$1 AND NOT deleted", chapterID).Scan(&projectID); err != nil {
		return Impact{}, noRows(err)
	}
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID)); err != nil {
		return Impact{}, err
	}
	c, err := scanChapter(tx.QueryRow(ctx, "SELECT "+chapterColumns+" FROM wb_chapters WHERE id=$1 AND NOT deleted FOR UPDATE", chapterID))
	if err != nil {
		return Impact{}, err
	}
	r, err := scanRevision(tx.QueryRow(ctx, "SELECT "+revisionColumns+" FROM wb_revisions WHERE id=$1 AND chapter_id=$2", revisionID, chapterID))
	if err != nil {
		return Impact{}, err
	}
	if r.ID != c.CurrentRevisionID {
		return Impact{}, &Conflict{Message: "只能定稿当前正文版本", CurrentRevisionID: c.CurrentRevisionID}
	}
	out := Impact{ProjectID: projectID, ChapterIDs: []string{}}
	if c.FinalRevisionID == revisionID {
		return out, nil
	}
	if c.FinalRevisionID != "" {
		out, err = s.invalidateTx(ctx, tx, projectID, c.Order, "已定稿正文换版")
		if err != nil {
			return out, err
		}
	} else {
		if _, err = tx.Exec(ctx, "UPDATE wb_projects SET epoch=epoch+1,health=CASE WHEN health='RebuildRequired' THEN health ELSE 'MemoryPending' END WHERE id=$1", projectID); err != nil {
			return out, err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_chapters SET final_revision_id=$2,needs_review=false WHERE id=$1", chapterID, revisionID); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) invalidateTx(ctx context.Context, tx pgx.Tx, projectID string, earliestOrder int, reason string) (Impact, error) {
	out := Impact{ID: job.NewID(), ProjectID: projectID, EarliestOrder: earliestOrder, Reason: reason, ChapterIDs: []string{}}
	rows, err := tx.Query(ctx, "SELECT id FROM wb_chapters WHERE project_id=$1 AND sort_order >=$2 AND NOT deleted ORDER BY sort_order", projectID, earliestOrder)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		out.ChapterIDs = append(out.ChapterIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, query := range []string{
		"UPDATE wb_chapters SET needs_review=true WHERE project_id=$1 AND sort_order >=$2 AND NOT deleted",
		"UPDATE wb_states SET valid=false WHERE project_id=$1 AND chapter_order >=$2",
		"UPDATE wb_facts SET active=false WHERE project_id=$1 AND chapter_order >=$2",
		"UPDATE wb_chunks SET active=false WHERE project_id=$1 AND chapter_order >=$2",
	} {
		if _, err = tx.Exec(ctx, query, projectID, earliestOrder); err != nil {
			return out, err
		}
	}
	// The most recent surviving state is the only permissible rebuild checkpoint.
	if _, err = tx.Exec(ctx, "UPDATE wb_projects SET epoch=epoch+1,health=CASE WHEN EXISTS(SELECT 1 FROM wb_chapters WHERE project_id=$1 AND NOT deleted AND final_revision_id<>'') THEN 'RebuildRequired' ELSE 'Ready' END,state_version_id=COALESCE((SELECT id FROM wb_states WHERE project_id=$1 AND valid ORDER BY chapter_order DESC,created_at DESC LIMIT 1),'') WHERE id=$1", projectID); err != nil {
		return out, err
	}
	ids, _ := json.Marshal(out.ChapterIDs)
	err = tx.QueryRow(ctx, "INSERT INTO wb_impacts(id,project_id,earliest_order,reason,chapter_ids) VALUES($1,$2,$3,$4,$5) RETURNING created_at", out.ID, projectID, earliestOrder, reason, ids).Scan(&out.CreatedAt)
	return out, err
}
func (s *Service) ReorderChapters(ctx context.Context, projectID string, expectedEpoch int64, chapterIDs []string) (Impact, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Impact{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID))
	if err != nil {
		return Impact{}, err
	}
	if p.Epoch != expectedEpoch {
		return Impact{}, &Conflict{Message: "作品顺序或状态已变化，请刷新"}
	}
	rows, err := tx.Query(ctx, "SELECT id,sort_order FROM wb_chapters WHERE project_id=$1 AND NOT deleted ORDER BY sort_order", projectID)
	if err != nil {
		return Impact{}, err
	}
	old := map[string]int{}
	for rows.Next() {
		var id string
		var order int
		if err = rows.Scan(&id, &order); err != nil {
			rows.Close()
			return Impact{}, err
		}
		old[id] = order
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Impact{}, err
	}
	if len(old) != len(chapterIDs) {
		return Impact{}, fmt.Errorf("排序必须包含全部活动章节")
	}
	seen := map[string]bool{}
	earliest := len(chapterIDs) + 1
	for i, id := range chapterIDs {
		order, ok := old[id]
		if !ok || seen[id] {
			return Impact{}, fmt.Errorf("排序包含未知、重复或其他作品章节")
		}
		seen[id] = true
		if order != i+1 {
			if i+1 < earliest {
				earliest = i + 1
			}
			if order < earliest {
				earliest = order
			}
		}
	}
	if earliest == len(chapterIDs)+1 {
		return Impact{ProjectID: projectID, ChapterIDs: []string{}}, nil
	}
	for i, id := range chapterIDs {
		if _, err = tx.Exec(ctx, "UPDATE wb_chapters SET sort_order=$2 WHERE id=$1", id, i+1); err != nil {
			return Impact{}, err
		}
	}
	out, err := s.invalidateTx(ctx, tx, projectID, earliest, "章节顺序变更")
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) DeleteChapter(ctx context.Context, id string) (Impact, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Impact{}, err
	}
	defer tx.Rollback(ctx)
	var projectID string
	if err = tx.QueryRow(ctx, "SELECT project_id FROM wb_chapters WHERE id=$1 AND NOT deleted", id).Scan(&projectID); err != nil {
		return Impact{}, noRows(err)
	}
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID)); err != nil {
		return Impact{}, err
	}
	c, err := scanChapter(tx.QueryRow(ctx, "SELECT "+chapterColumns+" FROM wb_chapters WHERE id=$1 AND NOT deleted FOR UPDATE", id))
	if err != nil {
		return Impact{}, err
	}
	out, err := s.invalidateTx(ctx, tx, projectID, c.Order, "章节已删除")
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_chapters SET deleted=true WHERE id=$1", id); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_projects SET health='Ready',state_version_id='' WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM wb_chapters WHERE project_id=$1 AND NOT deleted AND final_revision_id<>'')", projectID); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) ListImpacts(ctx context.Context, projectID string) ([]Impact, error) {
	rows, err := s.DB.Query(ctx, "SELECT id,project_id,earliest_order,reason,chapter_ids,created_at FROM wb_impacts WHERE project_id=$1 ORDER BY created_at DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Impact{}
	for rows.Next() {
		var v Impact
		var raw []byte
		if err = rows.Scan(&v.ID, &v.ProjectID, &v.EarliestOrder, &v.Reason, &raw, &v.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v.ChapterIDs); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
