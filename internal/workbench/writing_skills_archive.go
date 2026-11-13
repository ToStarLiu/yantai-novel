package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

// A book package preserves current author-selected guides. Private run logs and
// past skill revisions are part of instance backup, not a portable book.
func exportWritingSkillsTx(ctx context.Context, tx pgx.Tx, pid string) ([]WritingSkill, error) {
	out := []WritingSkill{}
	rows, err := tx.Query(ctx, "SELECT payload,id,project_id,version,enabled,priority,task_types FROM wb_writing_skills WHERE project_id=$1 AND NOT deleted ORDER BY priority DESC,name,id", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item WritingSkill
		var payload, tasks []byte
		if err = rows.Scan(&payload, &item.ID, &item.ProjectID, &item.Version, &item.Enabled, &item.Priority, &tasks); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(payload, &item.SkillPreview); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(tasks, &item.TaskTypes); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func importWritingSkillsTx(ctx context.Context, tx pgx.Tx, pid, sourceID string, items []WritingSkill) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) > 50 {
		return fmt.Errorf("作品包中的Skill超过50个")
	}
	names := map[string]bool{}
	ids := map[string]bool{}
	enabled := 0
	for _, item := range items {
		if item.ID == "" || ids[item.ID] || item.ProjectID != sourceID {
			return fmt.Errorf("作品包Skill来源或ID无效")
		}
		ids[item.ID] = true
		parsed, err := parseWritingSkill(item.Content, item.Resources, item.Source)
		if err != nil {
			return err
		}
		if names[parsed.Name] {
			return fmt.Errorf("作品包存在重复Skill名称")
		}
		names[parsed.Name] = true
		if item.Priority < -100 || item.Priority > 100 {
			return fmt.Errorf("作品包Skill优先级无效")
		}
		tasks, err := skillTaskTypes(item.TaskTypes)
		if err != nil {
			return err
		}
		if item.Enabled {
			enabled++
		}
		if enabled > 6 {
			return fmt.Errorf("每部作品最多启用6个Skill")
		}
		item.SkillPreview = parsed
		item.ID = job.NewID()
		item.ProjectID = pid
		item.Version = 1
		item.TaskTypes = tasks
		payload, _ := json.Marshal(parsed)
		taskJSON, _ := json.Marshal(tasks)
		history, _ := json.Marshal(item)
		if _, err = tx.Exec(ctx, "INSERT INTO wb_writing_skills(id,project_id,name,version,enabled,priority,task_types,payload,hash) VALUES($1,$2,$3,1,$4,$5,$6,$7,$8)", item.ID, pid, item.Name, item.Enabled, item.Priority, taskJSON, payload, item.Hash); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO wb_writing_skill_versions(skill_id,project_id,version,snapshot) VALUES($1,$2,1,$3)", item.ID, pid, history); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, "UPDATE wb_projects SET writing_skills_version=1 WHERE id=$1", pid)
	return err
}
