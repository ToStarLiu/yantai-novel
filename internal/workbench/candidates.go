package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"novel/internal/job"
	"novel/internal/review"
	"sort"
	"time"
)

type CandidateRequest struct {
	Count           int    `json:"count"`
	InputRevisionID string `json:"inputRevisionId"`
	Instructions    string `json:"instructions"`
}
type CandidateRanking struct {
	RunID       string         `json:"runId"`
	ReviewRunID string         `json:"reviewRunId"`
	Score       int            `json:"score"`
	Issues      []review.Issue `json:"issues"`
	Explanation string         `json:"explanation"`
}
type CandidateGroup struct {
	ID        string             `json:"id"`
	ProjectID string             `json:"projectId"`
	ChapterID string             `json:"chapterId"`
	Status    string             `json:"status"`
	Input     CandidateRequest   `json:"input"`
	RunIDs    []string           `json:"runIds"`
	Rankings  []CandidateRanking `json:"rankings"`
	CreatedAt time.Time          `json:"createdAt"`
}

func (s *Service) StartCandidates(ctx context.Context, cid string, in CandidateRequest) (CandidateGroup, error) {
	if in.Count != 2 && in.Count != 3 {
		return CandidateGroup{}, fmt.Errorf("高质量模式支持2或3个独立候选")
	}
	c, e := s.GetChapter(ctx, cid)
	if e != nil {
		return CandidateGroup{}, e
	}
	if in.InputRevisionID == "" {
		in.InputRevisionID = c.CurrentRevisionID
	}
	if c.CurrentRevisionID != in.InputRevisionID {
		return CandidateGroup{}, &Conflict{Message: "正文已变化"}
	}
	raw, _ := json.Marshal(in)
	id := job.NewID()
	_, e = s.DB.Exec(ctx, "INSERT INTO wb_candidate_groups(id,project_id,chapter_id,status,input) VALUES($1,$2,$3,'Running',$4)", id, c.ProjectID, cid, raw)
	if e != nil {
		return CandidateGroup{}, e
	}
	return s.GetCandidateGroup(ctx, id)
}
func (s *Service) GetCandidateGroup(ctx context.Context, id string) (CandidateGroup, error) {
	var g CandidateGroup
	var raw, runs, rank []byte
	e := s.DB.QueryRow(ctx, "SELECT id,project_id,chapter_id,status,input,run_ids,rankings,created_at FROM wb_candidate_groups WHERE id=$1", id).Scan(&g.ID, &g.ProjectID, &g.ChapterID, &g.Status, &raw, &runs, &rank, &g.CreatedAt)
	if e != nil {
		return g, noRows(e)
	}
	if e = json.Unmarshal(raw, &g.Input); e != nil {
		return g, e
	}
	if e = json.Unmarshal(runs, &g.RunIDs); e != nil {
		return g, e
	}
	e = json.Unmarshal(rank, &g.Rankings)
	return g, e
}
func (s *Service) ListCandidateGroups(ctx context.Context, pid string) ([]CandidateGroup, error) {
	rows, e := s.DB.Query(ctx, "SELECT id FROM wb_candidate_groups WHERE project_id=$1 ORDER BY created_at DESC LIMIT 100", pid)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []CandidateGroup{}
	for _, id := range ids {
		g, e := s.GetCandidateGroup(ctx, id)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, nil
}
func (s *Service) AdvanceCandidates(ctx context.Context) error {
	rows, e := s.DB.Query(ctx, "SELECT id FROM wb_candidate_groups WHERE status='Running' ORDER BY created_at LIMIT 10")
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if e = s.advanceCandidate(ctx, id); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) advanceCandidate(ctx context.Context, id string) error {
	g, e := s.GetCandidateGroup(ctx, id)
	if e != nil {
		return e
	}
	runs := []Run{}
	for i := 0; i < g.Input.Count; i++ {
		intent := fmt.Sprintf("candidate:%s:%d", g.ID, i)
		var runID string
		e = s.DB.QueryRow(ctx, "SELECT id FROM wb_runs WHERE intent_id=$1", intent).Scan(&runID)
		var r Run
		if e != nil {
			r, e = s.SubmitRun(ctx, g.ChapterID, RunRequest{IntentID: intent, TaskType: "draft", InputRevisionID: g.Input.InputRevisionID, Instructions: g.Input.Instructions + fmt.Sprintf("\n候选%d：提供一种独立叙事展开，保持所有事实约束。", i+1)})
			if e != nil {
				_, _ = s.DB.Exec(ctx, "UPDATE wb_candidate_groups SET status='Failed' WHERE id=$1", id)
				return nil
			}
		} else {
			r, e = s.GetRun(ctx, runID)
			if e != nil {
				return e
			}
		}
		runs = append(runs, r)
	}
	g.RunIDs = []string{}
	for _, r := range runs {
		g.RunIDs = append(g.RunIDs, r.ID)
	}
	raw, _ := json.Marshal(g.RunIDs)
	if _, e = s.DB.Exec(ctx, "UPDATE wb_candidate_groups SET run_ids=$2 WHERE id=$1", id, raw); e != nil {
		return e
	}
	rankings := []CandidateRanking{}
	ready := true
	for _, r := range runs {
		if r.Status == "Failed" || r.Status == "NeedsReconciliation" || r.Status == "Cancelled" || r.Stale {
			_, e = s.DB.Exec(ctx, "UPDATE wb_candidate_groups SET status='NeedsAttention' WHERE id=$1", id)
			return e
		}
		if r.Status != "Succeeded" {
			ready = false
			continue
		}
		intent := "candidate-review:" + g.ID + ":" + r.ID
		var rid string
		var reviewRun Run
		e = s.DB.QueryRow(ctx, "SELECT id FROM wb_runs WHERE intent_id=$1", intent).Scan(&rid)
		if e != nil {
			reviewRun, e = s.SubmitRun(ctx, g.ChapterID, RunRequest{IntentID: intent, TaskType: "review", InputRevisionID: g.Input.InputRevisionID, CandidateRunID: r.ID})
			if e != nil {
				_, _ = s.DB.Exec(ctx, "UPDATE wb_candidate_groups SET status='NeedsAttention' WHERE id=$1", id)
				return nil
			}
		} else {
			reviewRun, e = s.GetRun(ctx, rid)
			if e != nil {
				return e
			}
		}
		if reviewRun.Status == "Failed" || reviewRun.Status == "NeedsReconciliation" || reviewRun.Status == "Cancelled" || reviewRun.Stale {
			_, e = s.DB.Exec(ctx, "UPDATE wb_candidate_groups SET status='NeedsAttention' WHERE id=$1", id)
			return e
		}
		if reviewRun.Status != "Succeeded" {
			ready = false
			continue
		}
		score := 100
		for _, issue := range reviewRun.Output.Issues {
			switch issue.Severity {
			case "Critical":
				score -= 30
			case "Major":
				score -= 15
			default:
				score -= 5
			}
		}
		if score < 0 {
			score = 0
		}
		rankings = append(rankings, CandidateRanking{r.ID, reviewRun.ID, score, reviewRun.Output.Issues, "按带证据一致性问题扣分；不是文学质量总评，最终由作者选择。"})
	}
	sort.SliceStable(rankings, func(i, j int) bool { return rankings[i].Score > rankings[j].Score })
	raw, _ = json.Marshal(rankings)
	status := "Running"
	if ready && len(rankings) == len(runs) {
		status = "Succeeded"
	}
	_, e = s.DB.Exec(ctx, "UPDATE wb_candidate_groups SET status=$2,rankings=$3 WHERE id=$1", id, status, raw)
	return e
}
