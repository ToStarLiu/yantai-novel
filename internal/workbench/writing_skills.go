package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"novel/internal/job"
	"sort"
	"strings"
)

type WritingSkillResource struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
type WritingSkillSource struct {
	URL          string `json:"url"`
	Revision     string `json:"revision"`
	License      string `json:"license"`
	Author       string `json:"author"`
	Verification string `json:"verification,omitempty"`
}
type SkillPreview struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Content      string                 `json:"content"`
	Resources    []WritingSkillResource `json:"resources"`
	Source       WritingSkillSource     `json:"source"`
	Warnings     []string               `json:"warnings"`
	Hash         string                 `json:"hash"`
	PreviewToken string                 `json:"previewToken,omitempty"`
}
type WritingSkill struct {
	SkillPreview
	ID        string   `json:"id"`
	ProjectID string   `json:"projectId"`
	Version   int64    `json:"version"`
	Enabled   bool     `json:"enabled"`
	Priority  int      `json:"priority"`
	TaskTypes []string `json:"taskTypes"`
}
type WritingSkillList struct {
	Items   []WritingSkill `json:"items"`
	Version int64          `json:"version"`
}
type WritingSkillSave struct {
	Preview   SkillPreview `json:"preview"`
	Enabled   bool         `json:"enabled"`
	Priority  int          `json:"priority"`
	TaskTypes []string     `json:"taskTypes"`
}
type WritingSkillUpdate struct {
	ExpectedVersion int64         `json:"expectedVersion"`
	Content         *string       `json:"content,omitempty"`
	Preview         *SkillPreview `json:"preview,omitempty"`
	Enabled         *bool         `json:"enabled,omitempty"`
	Priority        *int          `json:"priority,omitempty"`
	TaskTypes       *[]string     `json:"taskTypes,omitempty"`
}
type WritingSkillInstruction struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     int64                  `json:"version"`
	Hash        string                 `json:"hash"`
	Content     string                 `json:"content"`
	Resources   []WritingSkillResource `json:"resources"`
}
type WritingSkillSnapshot struct {
	Version int64                     `json:"version"`
	Hash    string                    `json:"hash"`
	Items   []WritingSkillInstruction `json:"items"`
}
type WritingSkillResourcePage struct {
	SkillID    string `json:"skillId"`
	Name       string `json:"name"`
	Version    int64  `json:"version"`
	Hash       string `json:"hash"`
	Path       string `json:"path"`
	Text       string `json:"text"`
	Offset     int    `json:"offset"`
	NextOffset int    `json:"nextOffset"`
	TotalChars int    `json:"totalChars"`
	Truncated  bool   `json:"truncated"`
}

type skillRowDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func WritingSkillsVersion(ctx context.Context, db skillRowDB, pid string) (int64, error) {
	var v int64
	err := db.QueryRow(ctx, "SELECT writing_skills_version FROM wb_projects WHERE id=$1", pid).Scan(&v)
	return v, noRows(err)
}
func (s *Service) CurrentWritingSkillsVersion(ctx context.Context, pid string) (int64, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return 0, err
	}
	return WritingSkillsVersion(ctx, s.DB, pid)
}
func ValidateWritingSkillsTx(ctx context.Context, tx pgx.Tx, pid string, version int64) (bool, error) {
	v, err := WritingSkillsVersion(ctx, tx, pid)
	return v == version, err
}
func (s *Service) ListWritingSkills(ctx context.Context, pid string) (WritingSkillList, error) {
	out := WritingSkillList{Items: []WritingSkill{}}
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return out, err
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	out.Version, err = WritingSkillsVersion(ctx, tx, pid)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT payload,id,project_id,version,enabled,priority,task_types,hash FROM wb_writing_skills WHERE project_id=$1 AND NOT deleted ORDER BY priority DESC,name,id", pid)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v WritingSkill
		var raw, tasks []byte
		if err = rows.Scan(&raw, &v.ID, &v.ProjectID, &v.Version, &v.Enabled, &v.Priority, &tasks, &v.Hash); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &v.SkillPreview); err != nil {
			return out, err
		}
		if err = json.Unmarshal(tasks, &v.TaskTypes); err != nil {
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
func skillTaskTypes(tasks []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range tasks {
		if !advancedTask(v) && v != "copilot" {
			return nil, fmt.Errorf("Skill适用任务类型无效")
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out, nil
}
func (s *Service) SaveWritingSkill(ctx context.Context, pid string, in WritingSkillSave) (WritingSkill, error) {
	if err := s.RequireProjectRole(ctx, pid, "editor"); err != nil {
		return WritingSkill{}, err
	}
	verifiedSource := validWritingSkillPreview(in.Preview) && in.Preview.Source.Verification == "github-pinned"
	preview, err := parseWritingSkill(in.Preview.Content, in.Preview.Resources, in.Preview.Source)
	if err != nil {
		return WritingSkill{}, err
	}
	if verifiedSource {
		markWritingSkillVerified(&preview)
	}
	in.Preview = preview
	v := WritingSkill{SkillPreview: preview, ID: job.NewID(), ProjectID: pid, Version: 1, Enabled: in.Enabled, Priority: in.Priority, TaskTypes: in.TaskTypes}
	return s.persistWritingSkill(ctx, v, 0, false)
}
func (s *Service) GetWritingSkill(ctx context.Context, pid, id string) (WritingSkill, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return WritingSkill{}, err
	}
	var v WritingSkill
	var raw, tasks []byte
	err := s.DB.QueryRow(ctx, "SELECT payload,id,project_id,version,enabled,priority,task_types,hash FROM wb_writing_skills WHERE project_id=$1 AND id=$2 AND NOT deleted", pid, id).Scan(&raw, &v.ID, &v.ProjectID, &v.Version, &v.Enabled, &v.Priority, &tasks, &v.Hash)
	if err != nil {
		return v, noRows(err)
	}
	if err = json.Unmarshal(raw, &v.SkillPreview); err != nil {
		return v, err
	}
	err = json.Unmarshal(tasks, &v.TaskTypes)
	return v, err
}
func (s *Service) UpdateWritingSkill(ctx context.Context, pid, id string, in WritingSkillUpdate) (WritingSkill, error) {
	if err := s.RequireProjectRole(ctx, pid, "editor"); err != nil {
		return WritingSkill{}, err
	}
	v, err := s.GetWritingSkill(ctx, pid, id)
	if err != nil {
		return v, err
	}
	if in.ExpectedVersion != v.Version {
		return v, &Conflict{Message: "Skill已有新版本，请刷新后编辑"}
	}
	if in.Preview != nil && in.Content != nil {
		return v, fmt.Errorf("请只提交正文编辑或重新导入预览其中一种")
	}
	if in.Preview != nil {
		verified := validWritingSkillPreview(*in.Preview) && in.Preview.Source.Verification == "github-pinned"
		v.SkillPreview, err = parseWritingSkill(in.Preview.Content, in.Preview.Resources, in.Preview.Source)
		if err != nil {
			return v, err
		}
		if verified {
			markWritingSkillVerified(&v.SkillPreview)
		}
	}
	if in.Content != nil {
		v.SkillPreview, err = parseWritingSkill(*in.Content, v.Resources, v.Source)
		if err != nil {
			return v, err
		}
	}
	if in.Enabled != nil {
		v.Enabled = *in.Enabled
	}
	if in.Priority != nil {
		v.Priority = *in.Priority
	}
	if in.TaskTypes != nil {
		v.TaskTypes = *in.TaskTypes
	}
	v.Version++
	return s.persistWritingSkill(ctx, v, in.ExpectedVersion, false)
}
func (s *Service) DeleteWritingSkill(ctx context.Context, pid, id string, expected int64) error {
	if err := s.RequireProjectRole(ctx, pid, "editor"); err != nil {
		return err
	}
	v, err := s.GetWritingSkill(ctx, pid, id)
	if err != nil {
		return err
	}
	v.Version++
	v.Enabled = false
	_, err = s.persistWritingSkill(ctx, v, expected, true)
	return err
}
func (s *Service) persistWritingSkill(ctx context.Context, v WritingSkill, expected int64, deleted bool) (WritingSkill, error) {
	if v.Priority < -100 || v.Priority > 100 {
		return v, fmt.Errorf("Skill优先级应在-100至100")
	}
	var err error
	v.TaskTypes, err = skillTaskTypes(v.TaskTypes)
	if err != nil {
		return v, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	var version int64
	if err = tx.QueryRow(ctx, "SELECT writing_skills_version FROM wb_projects WHERE id=$1 FOR UPDATE", v.ProjectID).Scan(&version); err != nil {
		return v, noRows(err)
	}
	if expected == 0 {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM wb_writing_skills WHERE project_id=$1 AND NOT deleted", v.ProjectID).Scan(&count); err != nil {
			return v, err
		}
		if count >= 50 {
			return v, fmt.Errorf("每部作品最多保存50个Skill，请先整理不再使用的条目")
		}
	}
	if v.Enabled && !deleted {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM wb_writing_skills WHERE project_id=$1 AND enabled AND NOT deleted AND id<>$2", v.ProjectID, v.ID).Scan(&count); err != nil {
			return v, err
		}
		if count >= 6 {
			return v, fmt.Errorf("每部作品最多同时启用6个Skill，建议保留1至3个最相关的；其他条目可停用后保存")
		}
	}
	raw, _ := json.Marshal(v.SkillPreview)
	tasks, _ := json.Marshal(v.TaskTypes)
	if expected == 0 {
		_, err = tx.Exec(ctx, "INSERT INTO wb_writing_skills(id,project_id,name,version,enabled,priority,task_types,payload,hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", v.ID, v.ProjectID, v.Name, v.Version, v.Enabled, v.Priority, tasks, raw, v.Hash)
	} else {
		tag, e := tx.Exec(ctx, "UPDATE wb_writing_skills SET name=$3,version=$4,enabled=$5,priority=$6,task_types=$7,payload=$8,hash=$9,deleted=$10,updated_at=now() WHERE project_id=$1 AND id=$2 AND version=$11 AND NOT deleted", v.ProjectID, v.ID, v.Name, v.Version, v.Enabled, v.Priority, tasks, raw, v.Hash, deleted, expected)
		err = e
		if e == nil && tag.RowsAffected() != 1 {
			return v, &Conflict{Message: "Skill已变更，请刷新后操作"}
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return v, &Conflict{Message: "本书已有同名Skill，请编辑原条目或修改name后导入"}
		}
		return v, err
	}
	snapshot, _ := json.Marshal(v)
	if _, err = tx.Exec(ctx, "INSERT INTO wb_writing_skill_versions(skill_id,project_id,version,snapshot) VALUES($1,$2,$3,$4)", v.ID, v.ProjectID, v.Version, snapshot); err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_projects SET writing_skills_version=writing_skills_version+1 WHERE id=$1", v.ProjectID); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
func (s *Service) ResolveWritingSkills(ctx context.Context, pid, taskType string) (WritingSkillSnapshot, error) {
	list, err := s.ListWritingSkills(ctx, pid)
	out := WritingSkillSnapshot{Version: list.Version, Items: []WritingSkillInstruction{}}
	if err != nil {
		return out, err
	}
	for _, v := range list.Items {
		if !v.Enabled {
			continue
		}
		matches := len(v.TaskTypes) == 0
		for _, kind := range v.TaskTypes {
			matches = matches || kind == taskType
		}
		if !matches {
			continue
		}
		out.Items = append(out.Items, WritingSkillInstruction{ID: v.ID, Name: v.Name, Description: v.Description, Version: v.Version, Hash: v.Hash, Content: v.Content, Resources: v.Resources})
	}
	if len(out.Items) > 0 {
		raw, _ := json.Marshal(out.Items)
		out.Hash = job.Hash(raw)
	}
	return out, nil
}
func (s *Service) ReadWritingSkillResource(ctx context.Context, pid, sid string, expectedVersion int64, resourcePath string, offset, maxChars int) (WritingSkillResourcePage, error) {
	out := WritingSkillResourcePage{}
	v, err := s.GetWritingSkill(ctx, pid, sid)
	if err != nil {
		return out, err
	}
	if !v.Enabled || v.Version != expectedVersion {
		return out, &Conflict{Message: "Skill未启用或版本已更新，请重新读取当前配置"}
	}
	text := v.Content
	if resourcePath != "" && resourcePath != "SKILL.md" {
		found := false
		for _, r := range v.Resources {
			if r.Path == resourcePath {
				text = r.Content
				found = true
				break
			}
		}
		if !found {
			return out, ErrNotFound
		}
	} else {
		resourcePath = "SKILL.md"
	}
	if maxChars == 0 {
		maxChars = 2000
	}
	runes := []rune(text)
	if offset < 0 || offset > len(runes) || maxChars < 100 || maxChars > 3000 {
		return out, fmt.Errorf("Skill续读位置或长度无效")
	}
	end := min(len(runes), offset+maxChars)
	out = WritingSkillResourcePage{SkillID: v.ID, Name: v.Name, Version: v.Version, Hash: v.Hash, Path: resourcePath, Text: string(runes[offset:end]), Offset: offset, NextOffset: end, TotalChars: len(runes), Truncated: offset > 0 || end < len(runes)}
	return out, nil
}
