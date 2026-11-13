package workbench

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

type Query struct {
	ProjectID       string   `json:"projectId"`
	ProjectEpoch    int64    `json:"projectEpoch"`
	BeforeChapterID string   `json:"beforeChapterId"`
	CharacterID     string   `json:"characterId"`
	Purpose         string   `json:"purpose"`
	Text            string   `json:"text"`
	EntityIDs       []string `json:"entityIds"`
	Limit           int      `json:"limit"`
}
type Citation struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"projectId"`
	ChapterID    string  `json:"chapterId"`
	ChapterTitle string  `json:"chapterTitle"`
	RevisionID   string  `json:"revisionId"`
	ChapterOrder int     `json:"chapterOrder"`
	Content      string  `json:"content"`
	SourceHash   string  `json:"sourceHash"`
	Score        float64 `json:"score"`
	Reason       string  `json:"reason"`
	Kind         string  `json:"kind"`
	CharacterID  string  `json:"characterId,omitempty"`
	TruthStatus  string  `json:"truthStatus,omitempty"`
}

func chunks(text string) []string {
	result := []string{}
	for _, p := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		r := []rune(p)
		for len(r) > 500 {
			end := 500
			for j := 499; j >= 350; j-- {
				if strings.ContainsRune("。！？；", r[j]) {
					end = j + 1
					break
				}
			}
			result = append(result, string(r[:end]))
			r = r[end:]
		}
		if len(r) > 0 {
			result = append(result, string(r))
		}
	}
	return result
}
func indexChapterTx(ctx context.Context, tx pgx.Tx, projectID, chapterID string, order int, revisionID, hash, text string) error {
	if _, err := tx.Exec(ctx, "UPDATE wb_chunks SET active=false WHERE chapter_id=$1", chapterID); err != nil {
		return err
	}
	for i, chunk := range chunks(text) {
		id := job.Hash([]byte(revisionID + fmt.Sprint(":", i)))
		_, err := tx.Exec(ctx, `INSERT INTO wb_chunks(id,project_id,chapter_id,chapter_order,revision_id,source_hash,chunk_order,content,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8,true) ON CONFLICT(revision_id,chunk_order) DO UPDATE SET content=EXCLUDED.content,source_hash=EXCLUDED.source_hash,chapter_order=EXCLUDED.chapter_order,active=true`, id, projectID, chapterID, order, revisionID, hash, i, chunk)
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Reindex(ctx context.Context, projectID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR UPDATE", projectID).Scan(&id); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT c.id,c.sort_order,r.id,r.hash,r.plain_text FROM wb_chapters c JOIN wb_revisions r ON r.id=c.final_revision_id WHERE c.project_id=$1 AND NOT c.deleted AND NOT c.needs_review AND EXISTS(SELECT 1 FROM wb_states st WHERE st.project_id=c.project_id AND st.chapter_id=c.id AND st.source_revision_id=c.final_revision_id AND st.valid) ORDER BY c.sort_order`, projectID)
	if err != nil {
		return err
	}
	type source struct {
		id                   string
		order                int
		revision, hash, text string
	}
	sources := []source{}
	for rows.Next() {
		var row source
		if err = rows.Scan(&row.id, &row.order, &row.revision, &row.hash, &row.text); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_chunks SET active=false WHERE project_id=$1", projectID); err != nil {
		return err
	}
	for _, src := range sources {
		if err = indexChapterTx(ctx, tx, projectID, src.id, src.order, src.revision, src.hash, src.text); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func tokenize(text string) []string {
	unique := map[string]bool{}
	var word []rune
	var han []rune
	flushWord := func() {
		if len(word) > 0 {
			unique[string(word)] = true
			word = nil
		}
	}
	flushHan := func() {
		if len(han) == 1 {
			unique[string(han)] = true
		}
		for i := 0; i+1 < len(han); i++ {
			unique[string(han[i:i+2])] = true
		}
		han = nil
	}
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) {
			flushWord()
			han = append(han, r)
		} else if unicode.IsLetter(r) || unicode.IsNumber(r) {
			flushHan()
			word = append(word, r)
		} else {
			flushWord()
			flushHan()
		}
	}
	flushWord()
	flushHan()
	result := []string{}
	for term := range unique {
		result = append(result, term)
	}
	sort.Strings(result)
	return result
}
func relevance(content, query string, terms []string, entityNames []string) (float64, string) {
	score := 0.0
	lower := strings.ToLower(content)
	reasons := []string{}
	if query != "" && strings.Contains(lower, strings.ToLower(strings.TrimSpace(query))) {
		score += 10
		reasons = append(reasons, "原文短语匹配")
	}
	count := 0
	for _, term := range terms {
		if strings.Contains(lower, term) {
			score++
			count++
		}
	}
	if count > 0 {
		reasons = append(reasons, fmt.Sprintf("关键词命中%d项", count))
	}
	for _, name := range entityNames {
		if name != "" && strings.Contains(content, name) {
			score += 5
			reasons = append(reasons, "实体匹配："+name)
		}
	}
	return score, strings.Join(reasons, "；")
}

// Search performs hard visibility filtering before scoring. Character knowledge
// searches use only explicitly acquired facts, never narrator-level prose.
func (s *Service) Search(ctx context.Context, q Query) ([]Citation, error) {
	if q.ProjectID == "" {
		return nil, fmt.Errorf("缺少作品ID")
	}
	if q.Purpose == "" {
		q.Purpose = "AuthorReview"
	}
	if q.Purpose != "AuthorReview" && q.Purpose != "CharacterKnowledge" {
		return nil, fmt.Errorf("检索用途无效")
	}
	if q.Limit <= 0 {
		q.Limit = 10
	}
	if q.Limit > 50 {
		q.Limit = 50
	}
	if len(q.Text) > 2000 || len(q.EntityIDs) > 100 {
		return nil, fmt.Errorf("检索查询过大")
	}
	// A single read-only repeatable snapshot means a concurrent invalidation
	// cannot combine the new chapter pointer with an old index visibility flag.
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var epoch int64
	if err = tx.QueryRow(ctx, "SELECT epoch FROM wb_projects WHERE id=$1", q.ProjectID).Scan(&epoch); err != nil {
		return nil, err
	}
	if q.ProjectEpoch != 0 && q.ProjectEpoch != epoch {
		return nil, &Conflict{Message: "作品版本已变化，请刷新检索"}
	}
	before := 2147483647
	if q.BeforeChapterID != "" {
		if err = tx.QueryRow(ctx, "SELECT sort_order FROM wb_chapters WHERE id=$1 AND project_id=$2 AND NOT deleted", q.BeforeChapterID, q.ProjectID).Scan(&before); err != nil {
			return nil, fmt.Errorf("检索边界章节不属于当前作品")
		}
	}
	if q.Purpose == "CharacterKnowledge" {
		var exists bool
		if q.CharacterID == "" {
			return nil, fmt.Errorf("角色知识检索需要指定角色")
		}
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_settings WHERE id=$1 AND project_id=$2 AND kind='character')", q.CharacterID, q.ProjectID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("角色不属于当前作品")
		}
	}
	entityNames := []string{}
	for _, id := range q.EntityIDs {
		var name, content string
		if err = tx.QueryRow(ctx, "SELECT name,content FROM wb_settings WHERE id=$1 AND project_id=$2", id, q.ProjectID).Scan(&name, &content); err != nil {
			return nil, fmt.Errorf("检索实体不属于当前作品")
		}
		entityNames = append(entityNames, name)
		for _, alias := range strings.FieldsFunc(content, func(r rune) bool { return r == '、' || r == ',' || r == '，' || r == '\n' }) {
			alias = strings.TrimSpace(alias)
			if strings.HasPrefix(alias, "别名：") {
				entityNames = append(entityNames, strings.TrimPrefix(alias, "别名："))
			}
		}
	}
	terms := tokenize(q.Text)
	result := []Citation{}
	if len(terms) == 0 && len(entityNames) == 0 {
		return result, nil
	}
	if q.Purpose == "CharacterKnowledge" {
		facts, err := factsAt(ctx, tx, q.ProjectID, before-1)
		if err != nil {
			return nil, err
		}
		for _, f := range facts {
			if f.Kind != "Knowledge" || f.CharacterID != q.CharacterID {
				continue
			}
			// Evidence may quote a paragraph containing narrator-only information.
			// Only the explicitly acquired value belongs in a character projection.
			content := f.Object
			score, reason := relevance(content, q.Text, terms, entityNames)
			if score == 0 {
				continue
			}
			result = append(result, Citation{ID: f.ID, ProjectID: q.ProjectID, ChapterID: f.ChapterID, RevisionID: f.SourceRevisionID, ChapterOrder: f.ChapterOrder, Content: content, SourceHash: f.Evidence.PlainTextHash, Score: score, Reason: reason, Kind: "Knowledge", CharacterID: f.CharacterID, TruthStatus: f.TruthStatus})
		}
	} else {
		rows, err := tx.Query(ctx, `SELECT k.id,k.chapter_id,c.title,k.revision_id,c.sort_order,k.content,k.source_hash FROM wb_chunks k JOIN wb_chapters c ON c.id=k.chapter_id AND c.project_id=k.project_id AND NOT c.deleted AND NOT c.needs_review AND c.final_revision_id=k.revision_id JOIN wb_revisions r ON r.id=k.revision_id AND r.hash=k.source_hash WHERE k.project_id=$1 AND k.active AND c.sort_order<$2 AND EXISTS(SELECT 1 FROM wb_states st WHERE st.project_id=k.project_id AND st.chapter_id=c.id AND st.source_revision_id=k.revision_id AND st.valid) ORDER BY c.sort_order,k.chunk_order LIMIT 10000`, q.ProjectID, before)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c Citation
			c.ProjectID = q.ProjectID
			c.Kind = "Passage"
			if err = rows.Scan(&c.ID, &c.ChapterID, &c.ChapterTitle, &c.RevisionID, &c.ChapterOrder, &c.Content, &c.SourceHash); err != nil {
				rows.Close()
				return nil, err
			}
			c.Score, c.Reason = relevance(c.Content, q.Text, terms, entityNames)
			if c.Score > 0 {
				result = append(result, c)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].ID < result[j].ID
	})
	if len(result) > q.Limit {
		result = result[:q.Limit]
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// ConfirmedContext exposes only confirmed facts before the requested chapter;
// belief rows retain their role-specific truth status. With characterID set,
// only that character's acquired Knowledge is returned.
func (s *Service) ConfirmedContext(ctx context.Context, projectID, beforeChapterID, characterID string) (string, error) {
	before := 2147483647
	if beforeChapterID != "" {
		if err := s.DB.QueryRow(ctx, "SELECT sort_order FROM wb_chapters WHERE id=$1 AND project_id=$2 AND NOT deleted", beforeChapterID, projectID).Scan(&before); err != nil {
			return "", fmt.Errorf("章节不属于该作品")
		}
	}
	facts, err := factsAt(ctx, s.DB, projectID, before-1)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, f := range facts {
		if characterID != "" && (f.Kind != "Knowledge" || f.CharacterID != characterID) {
			continue
		}
		fmt.Fprintf(&b, "[%s] subject=%s %s=%s", f.Kind, f.Subject, f.Predicate, f.Object)
		if f.Kind == "Knowledge" {
			fmt.Fprintf(&b, " (仅角色%s的信念，truthStatus=%s)", f.CharacterID, f.TruthStatus)
		}
		if characterID == "" {
			fmt.Fprintf(&b, " [source=%s quote=%q]\n", f.SourceRevisionID, f.Evidence.Quote)
		} else {
			fmt.Fprintf(&b, " [source=%s]\n", f.SourceRevisionID)
		}
	}
	return b.String(), nil
}
