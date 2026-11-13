package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"novel/internal/job"
	"time"
)

// Activity is a readable historical artifact. Imported entries are never put in
// wb_runs, productions, candidate or media queues, even when their old status says Running.
type ProjectActivityArchive struct {
	SchemaVersion        int               `json:"schemaVersion"`
	ReadOnly             bool              `json:"readOnly"`
	SourceProjectID      string            `json:"sourceProjectId"`
	Runs                 []json.RawMessage `json:"runs"`
	Productions          []json.RawMessage `json:"productions"`
	Candidates           []json.RawMessage `json:"candidates"`
	Media                []json.RawMessage `json:"media"`
	Branches             []json.RawMessage `json:"branches"`
	Preferences          json.RawMessage   `json:"preferences,omitempty"`
	Prior                []json.RawMessage `json:"prior,omitempty"`
	MediaContentIncluded bool              `json:"mediaContentIncluded"`
	Note                 string            `json:"note"`
}
type ProjectArchiveView struct {
	Activity ProjectActivityArchive `json:"activity"`
	Imported bool                   `json:"imported"`
}

func archiveRows(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]json.RawMessage, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}
func (s *Service) exportActivityTx(ctx context.Context, tx pgx.Tx, pid string) (ProjectActivityArchive, error) {
	out := ProjectActivityArchive{SchemaVersion: 1, ReadOnly: true, SourceProjectID: pid, Note: "只读活动档案，不恢复可执行队列、账号或权限。媒体只有元数据，素材请单独下载或使用全库备份。"}
	var err error
	if out.Runs, err = archiveRows(ctx, tx, `SELECT jsonb_build_object('id',r.id,'chapterId',r.chapter_id,'revisionId',r.revision_id,'type',r.type,'status',r.status,'input',r.input,'output',r.output,'usage',r.usage,'cost',r.cost,'costStatus',r.cost_status,'createdAt',r.created_at,'finishedAt',r.finished_at,'stale',r.stale,'attempts',COALESCE((SELECT jsonb_agg(jsonb_build_object('ordinal',a.ordinal,'status',a.status,'disposition',a.disposition,'usage',a.usage,'startedAt',a.started_at,'finishedAt',a.finished_at) ORDER BY a.ordinal) FROM wb_run_attempts a WHERE a.run_id=r.id),'[]'::jsonb)) FROM wb_runs r WHERE project_id=$1 ORDER BY created_at,id`, pid); err != nil {
		return out, err
	}
	if out.Productions, err = archiveRows(ctx, tx, `SELECT jsonb_build_object('id',p.id,'workflow',p.workflow_snapshot,'chapterIds',p.chapter_ids,'maxCost',p.max_cost,'currency',p.currency,'manualGates',p.manual_gates,'status',p.status,'createdAt',p.created_at,'finishedAt',p.finished_at,'steps',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',s.id,'chapterId',s.chapter_id,'stepId',s.step_id,'order',s.sort_order,'status',s.status,'taskRunId',s.task_run_id,'approved',s.approved) ORDER BY sort_order,id) FROM wb_production_steps s WHERE s.production_id=p.id),'[]'::jsonb)) FROM wb_productions p WHERE project_id=$1 ORDER BY created_at,id`, pid); err != nil {
		return out, err
	}
	if out.Candidates, err = archiveRows(ctx, tx, `SELECT jsonb_build_object('id',id,'chapterId',chapter_id,'status',status,'input',input,'runIds',run_ids,'reviewIds',review_ids,'rankings',rankings,'createdAt',created_at) FROM wb_candidate_groups WHERE project_id=$1 ORDER BY created_at,id`, pid); err != nil {
		return out, err
	}
	if out.Media, err = archiveRows(ctx, tx, `SELECT jsonb_build_object('id',id,'chapterId',chapter_id,'revisionId',revision_id,'kind',kind,'status',status,'input',input,'mime',mime,'usage',usage,'costStatus',cost_status,'createdAt',created_at,'finishedAt',finished_at,'contentIncluded',false) FROM wb_media WHERE project_id=$1 ORDER BY created_at,id`, pid); err != nil {
		return out, err
	}
	branchSQL := `SELECT jsonb_build_object('id',id,'sourceProjectId',source_project_id,'targetProjectId',target_project_id,'sourceEpoch',source_epoch,'sourceTitle',source_title,'title',title,'createdAt',created_at) FROM wb_branch_links WHERE (source_project_id=$1 OR target_project_id=$1)`
	branchArgs := []any{pid}
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin {
		branchSQL += " AND source_project_id IN (SELECT project_id FROM wb_project_members WHERE user_id=$2) AND target_project_id IN (SELECT project_id FROM wb_project_members WHERE user_id=$2)"
		branchArgs = append(branchArgs, p.ID)
	}
	if out.Branches, err = archiveRows(ctx, tx, branchSQL+" ORDER BY created_at,id", branchArgs...); err != nil {
		return out, err
	}
	rows, err := archiveRows(ctx, tx, `SELECT jsonb_build_object('maxCost',max_cost,'currency',currency,'maxOutputTokens',max_output_tokens) FROM wb_preferences WHERE project_id=$1`, pid)
	if err != nil {
		return out, err
	}
	if len(rows) > 0 {
		out.Preferences = rows[0]
	}
	prior, err := archiveRows(ctx, tx, "SELECT activity FROM wb_project_archives WHERE project_id=$1", pid)
	if err != nil {
		return out, err
	}
	out.Prior = prior
	return out, nil
}
func (s *Service) ProjectArchive(ctx context.Context, pid string) (ProjectArchiveView, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return ProjectArchiveView{}, err
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ProjectArchiveView{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1", pid)); err != nil {
		return ProjectArchiveView{}, err
	}
	v, err := s.exportActivityTx(ctx, tx, pid)
	if err != nil {
		return ProjectArchiveView{}, err
	}
	return ProjectArchiveView{Activity: v, Imported: len(v.Prior) > 0}, tx.Commit(ctx)
}
func (s *Service) exportResearchTx(ctx context.Context, tx pgx.Tx, pid string) ([]ResearchReport, error) {
	rows, err := tx.Query(ctx, "SELECT id,project_id,topic,status,sources,created_at FROM wb_research_reports WHERE project_id=$1 ORDER BY created_at,id", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResearchReport{}
	for rows.Next() {
		var v ResearchReport
		var raw []byte
		if err = rows.Scan(&v.ID, &v.ProjectID, &v.Topic, &v.Status, &raw, &v.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v.Sources); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) importExtraArchiveTx(ctx context.Context, tx pgx.Tx, pid string, p ProjectPackage, ids map[string]string) error {
	if len(p.Research) > 1000 {
		return fmt.Errorf("研究报告超过上限")
	}
	for _, v := range p.Research {
		if v.ProjectID != p.Project.ID || v.ID == "" || len(v.Sources) > 5 {
			return fmt.Errorf("研究归档来源无效")
		}
		if _, ok := ids[v.ID]; ok {
			return fmt.Errorf("研究归档ID重复")
		}
		ids[v.ID] = job.NewID()
		if v.Status != "Success" && v.Status != "Partial" && v.Status != "Failed" {
			return fmt.Errorf("研究报告状态无效")
		}
		for i, source := range v.Sources {
			if source.ReferenceID != "" {
				mapped, ok := ids[source.ReferenceID]
				if !ok {
					return fmt.Errorf("研究来源的参考文档不在作品包内")
				}
				var present bool
				if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_studio_documents WHERE project_id=$1 AND kind='reference' AND id=$2)", pid, mapped).Scan(&present); err != nil {
					return err
				}
				if !present {
					return fmt.Errorf("研究来源不是有效参考文档")
				}
				v.Sources[i].ReferenceID = mapped
			}
		}
		raw, err := json.Marshal(v.Sources)
		if err != nil {
			return err
		}
		if v.CreatedAt.IsZero() {
			v.CreatedAt = time.Now().UTC()
		}
		if _, err = tx.Exec(ctx, "INSERT INTO wb_research_reports(id,project_id,topic,status,sources,created_at) VALUES($1,$2,$3,$4,$5,$6)", ids[v.ID], pid, v.Topic, v.Status, raw, v.CreatedAt); err != nil {
			return err
		}
	}
	if len(p.Activity) == 0 || string(p.Activity) == "null" {
		return nil
	}
	var activity ProjectActivityArchive
	if err := json.Unmarshal(p.Activity, &activity); err != nil || activity.SchemaVersion != 1 || activity.SourceProjectID != p.Project.ID {
		return fmt.Errorf("活动历史来源或版本无效")
	}
	activity.ReadOnly = true
	activity.MediaContentIncluded = false
	activity.Note = "只读导入历史，不创建任务、Provider调用、成员权限或媒体素材。历史来源ID保留在原作品命名空间。"
	raw, err := json.Marshal(activity)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO wb_project_archives(project_id,activity) VALUES($1,$2)", pid, raw)
	return err
}
