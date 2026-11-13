package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

const MaxProjectPackageBytes = 32 << 20

type ProjectPackage struct {
	FormatVersion int              `json:"formatVersion"`
	Project       Project          `json:"project"`
	Settings      []Setting        `json:"settings"`
	Chapters      []Chapter        `json:"chapters"`
	Revisions     []Revision       `json:"revisions"`
	Narrative     json.RawMessage  `json:"narrative,omitempty"`
	Studio        json.RawMessage  `json:"studio,omitempty"`
	Research      []ResearchReport `json:"research,omitempty"`
	Activity      json.RawMessage  `json:"activity,omitempty"`
	WritingSkills []WritingSkill   `json:"writingSkills,omitempty"`
}
type ImportChapter struct {
	Title  string `json:"title"`
	Volume string `json:"volume"`
	Text   string `json:"text"`
}

// ExportProject freezes a consistent, readable snapshot. It contains no model
// credentials, login sessions, queue entries, or executable jobs.
func (s *Service) ExportProject(ctx context.Context, id string) (ProjectPackage, error) {
	out := ProjectPackage{FormatVersion: 1, Settings: []Setting{}, Chapters: []Chapter{}, Revisions: []Revision{}}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.Project, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1", id))
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT id,project_id,kind,name,content FROM wb_settings WHERE project_id=$1 ORDER BY id", id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v Setting
		if err = rows.Scan(&v.ID, &v.ProjectID, &v.Kind, &v.Name, &v.Content); err != nil {
			rows.Close()
			return out, err
		}
		out.Settings = append(out.Settings, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, "SELECT "+chapterColumns+" FROM wb_chapters WHERE project_id=$1 ORDER BY sort_order,id", id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanChapter(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Chapters = append(out.Chapters, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, "SELECT "+prefixColumns(revisionColumns, "r")+" FROM wb_revisions r JOIN wb_chapters c ON c.id=r.chapter_id WHERE c.project_id=$1 ORDER BY r.created_at,r.id", id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanRevision(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Revisions = append(out.Revisions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	// Keep narrative data in this same snapshot transaction through its archive hook.
	out.Narrative, err = s.exportNarrativeTx(ctx, tx, id)
	if err != nil {
		return out, err
	}
	out.Studio, err = s.exportStudioTx(ctx, tx, id)
	if err != nil {
		return out, err
	}
	out.Research, err = s.exportResearchTx(ctx, tx, id)
	if err != nil {
		return out, err
	}
	activity, err := s.exportActivityTx(ctx, tx, id)
	if err != nil {
		return out, err
	}
	out.Activity, err = json.Marshal(activity)
	if err != nil {
		return out, err
	}
	out.WritingSkills, err = exportWritingSkillsTx(ctx, tx, id)
	if err != nil {
		return out, err
	}
	if raw, e := json.Marshal(out); e != nil || len(raw) > MaxProjectPackageBytes {
		return ProjectPackage{}, fmt.Errorf("作品包超过32MiB，请先单独保存活动历史/媒体，使用全库备份迁移大型作品")
	}
	return out, tx.Commit(ctx)
}
func prefixColumns(columns, prefix string) string {
	v := strings.Split(columns, ",")
	for i := range v {
		v[i] = prefix + "." + v[i]
	}
	return strings.Join(v, ",")
}

func validatePackage(p ProjectPackage) error {
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > MaxProjectPackageBytes {
		return fmt.Errorf("作品包无效或超过32MiB限制")
	}
	if p.FormatVersion != 1 {
		return fmt.Errorf("不支持的作品包版本")
	}
	if !validName(p.Project.Title) || p.Project.ID == "" || len(p.Chapters) > 2000 || len(p.Revisions) > 50000 || len(p.Settings) > 10000 {
		return fmt.Errorf("作品包结构或规模无效")
	}
	ids := map[string]bool{p.Project.ID: true}
	chapters := map[string]Chapter{}
	revisions := map[string]Revision{}
	register := func(id string) error {
		if id == "" || ids[id] || len(id) > 256 {
			return fmt.Errorf("作品包包含空或重复ID")
		}
		ids[id] = true
		return nil
	}
	for _, v := range p.Settings {
		if err = register(v.ID); err != nil {
			return err
		}
		if v.ProjectID != p.Project.ID || !validName(v.Name) {
			return fmt.Errorf("设定来源不匹配")
		}
		switch v.Kind {
		case "character", "world", "rule", "style", "outline":
		default:
			return fmt.Errorf("无效设定类型")
		}
	}
	for _, v := range p.Chapters {
		if err = register(v.ID); err != nil {
			return err
		}
		if v.ProjectID != p.Project.ID || !validName(v.Title) || v.Order < 1 {
			return fmt.Errorf("章节来源或顺序无效")
		}
		chapters[v.ID] = v
	}
	for _, r := range p.Revisions {
		if err = register(r.ID); err != nil {
			return err
		}
		if _, ok := chapters[r.ChapterID]; !ok {
			return fmt.Errorf("版本不属于包内章节")
		}
		_, plain, hash, e := NormalizeDocument(r.Document)
		if e != nil {
			return e
		}
		if plain != r.PlainText || hash != r.Hash {
			return fmt.Errorf("正文派生文本或哈希不匹配")
		}
		revisions[r.ID] = r
	}
	for _, r := range p.Revisions {
		if r.ParentID != "" {
			parent, ok := revisions[r.ParentID]
			if !ok || parent.ChapterID != r.ChapterID {
				return fmt.Errorf("版本父来源不匹配")
			}
		}
	}
	for _, c := range p.Chapters {
		for _, id := range []string{c.CurrentRevisionID, c.FinalRevisionID} {
			if id != "" {
				r, ok := revisions[id]
				if !ok || r.ChapterID != c.ID {
					return fmt.Errorf("章节活动版本不匹配")
				}
			}
		}
	}
	// Linear graph traversal avoids quadratic work for a long revision history.
	visited := map[string]bool{}
	for _, r := range p.Revisions {
		path := map[string]bool{}
		id := r.ID
		for id != "" && !visited[id] {
			if path[id] {
				return fmt.Errorf("版本来源存在循环")
			}
			path[id] = true
			id = revisions[id].ParentID
		}
		for id := range path {
			visited[id] = true
		}
	}
	return nil
}

func (s *Service) ImportProject(ctx context.Context, p ProjectPackage) (ProjectDetail, error) {
	if err := validatePackage(p); err != nil {
		return ProjectDetail{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ProjectDetail{}, err
	}
	defer tx.Rollback(ctx)
	projectID := job.NewID()
	idMap := map[string]string{p.Project.ID: projectID}
	for _, v := range p.Settings {
		idMap[v.ID] = job.NewID()
	}
	for _, c := range p.Chapters {
		idMap[c.ID] = job.NewID()
	}
	for _, r := range p.Revisions {
		idMap[r.ID] = job.NewID()
	}
	_, err = tx.Exec(ctx, "INSERT INTO wb_projects(id,title,description,genre,epoch,health) VALUES($1,$2,$3,$4,1,'Ready')", projectID, p.Project.Title, p.Project.Description, p.Project.Genre)
	if err != nil {
		return ProjectDetail{}, err
	}
	if err = ownerForNewProjectTx(ctx, tx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	for _, v := range p.Settings {
		_, err = tx.Exec(ctx, "INSERT INTO wb_settings(id,project_id,kind,name,content) VALUES($1,$2,$3,$4,$5)", idMap[v.ID], projectID, v.Kind, v.Name, v.Content)
		if err != nil {
			return ProjectDetail{}, err
		}
	}
	for _, c := range p.Chapters {
		_, err = tx.Exec(ctx, "INSERT INTO wb_chapters(id,project_id,volume,title,sort_order,blueprint,blueprint_version,current_revision_id,final_revision_id,needs_review,deleted) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)", idMap[c.ID], projectID, c.Volume, c.Title, c.Order, c.Blueprint, c.BlueprintVersion, idMap[c.CurrentRevisionID], idMap[c.FinalRevisionID], c.NeedsReview, c.Deleted)
		if err != nil {
			return ProjectDetail{}, err
		}
	}
	for _, r := range p.Revisions {
		// Archived run IDs remain historical provenance, never live task references.
		runID := ""
		if r.RunID != "" {
			runID = "archive:" + r.RunID
		}
		_, err = tx.Exec(ctx, "INSERT INTO wb_revisions(id,chapter_id,parent_id,document,plain_text,hash,source,run_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", idMap[r.ID], idMap[r.ChapterID], idMap[r.ParentID], r.Document, r.PlainText, r.Hash, r.Source, runID, r.CreatedAt)
		if err != nil {
			return ProjectDetail{}, err
		}
	}
	if len(p.Narrative) > 0 && string(p.Narrative) != "null" {
		if err = s.ImportNarrativeTx(ctx, tx, projectID, p.Narrative, idMap); err != nil {
			return ProjectDetail{}, err
		}
	} else {
		_, err = tx.Exec(ctx, "UPDATE wb_projects SET health=CASE WHEN EXISTS(SELECT 1 FROM wb_chapters WHERE project_id=$1 AND final_revision_id<>'' AND NOT deleted) THEN 'MemoryPending' ELSE 'Ready' END WHERE id=$1", projectID)
		if err != nil {
			return ProjectDetail{}, err
		}
	}
	if err = s.ImportStudioTx(ctx, tx, projectID, p.Studio, idMap); err != nil {
		return ProjectDetail{}, err
	}
	if err = s.importExtraArchiveTx(ctx, tx, projectID, p, idMap); err != nil {
		return ProjectDetail{}, err
	}
	if err = importWritingSkillsTx(ctx, tx, projectID, p.Project.ID, p.WritingSkills); err != nil {
		return ProjectDetail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ProjectDetail{}, err
	}
	return s.GetProject(ctx, projectID)
}

func (s *Service) ExportMarkdown(ctx context.Context, projectID string) (string, error) {
	p, err := s.ExportProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	revisions := map[string]Revision{}
	for _, r := range p.Revisions {
		revisions[r.ID] = r
	}
	sort.SliceStable(p.Chapters, func(i, j int) bool { return p.Chapters[i].Order < p.Chapters[j].Order })
	var b strings.Builder
	b.WriteString("# " + p.Project.Title + "\n\n")
	volume := ""
	for _, c := range p.Chapters {
		if c.Deleted {
			continue
		}
		if c.Volume != "" && c.Volume != volume {
			volume = c.Volume
			b.WriteString("# " + volume + "\n\n")
		}
		b.WriteString("## " + c.Title + "\n\n")
		b.WriteString(revisions[c.CurrentRevisionID].PlainText)
		b.WriteString("\n\n")
	}
	return b.String(), nil
}

var chapterHeading = regexp.MustCompile(`^(?:#{1,6}\s+(.+)|第[零一二三四五六七八九十百千万两〇0-9]+[章节回卷部集]\s*.*)$`)

func PreviewTextImport(text string) ([]ImportChapter, error) {
	if len(text) > MaxProjectPackageBytes || !utf8.ValidString(text) {
		return nil, fmt.Errorf("导入文本无效或超过32MiB")
	}
	text = strings.TrimPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "\ufeff")
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("导入正文为空")
	}
	out := []ImportChapter{}
	title := "导入章节"
	var lines []string
	flush := func() {
		body := strings.TrimSpace(strings.Join(lines, "\n"))
		if body != "" {
			out = append(out, ImportChapter{Title: title, Text: body})
		}
		lines = nil
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if chapterHeading.MatchString(trimmed) {
			flush()
			title = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		lines = append(lines, line)
	}
	flush()
	if len(out) == 0 {
		return nil, fmt.Errorf("未发现正文；请检查标题与分章")
	}
	if len(out) > 1000 {
		return nil, fmt.Errorf("导入章节数量超过1000")
	}
	return out, nil
}
func (s *Service) ImportText(ctx context.Context, projectID string, chapters []ImportChapter) ([]Chapter, error) {
	if len(chapters) == 0 || len(chapters) > 1000 {
		return nil, fmt.Errorf("请先预览并确认导入章节")
	}
	total := 0
	for _, c := range chapters {
		total += len(c.Text)
		if !validName(c.Title) || len(c.Volume) > 1000 || len(c.Text) > 4<<20 {
			return nil, fmt.Errorf("导入章节无效")
		}
	}
	if total > MaxProjectPackageBytes {
		return nil, fmt.Errorf("导入正文超过32MiB")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", projectID)); err != nil {
		return nil, err
	}
	var order int
	if err = tx.QueryRow(ctx, "SELECT COALESCE(max(sort_order),0) FROM wb_chapters WHERE project_id=$1", projectID).Scan(&order); err != nil {
		return nil, err
	}
	out := []Chapter{}
	for _, v := range chapters {
		order++
		c, e := scanChapter(tx.QueryRow(ctx, "INSERT INTO wb_chapters(id,project_id,volume,title,sort_order) VALUES($1,$2,$3,$4,$5) RETURNING "+chapterColumns, job.NewID(), projectID, v.Volume, v.Title, order))
		if e != nil {
			return nil, e
		}
		r, e := saveRevisionTx(ctx, tx, c, SaveInput{Document: DocumentFromText(v.Text), Source: "import"})
		if e != nil {
			return nil, e
		}
		c.CurrentRevisionID = r.ID
		c.Revision = &r
		out = append(out, c)
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_projects SET epoch=epoch+1 WHERE id=$1", projectID); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}
