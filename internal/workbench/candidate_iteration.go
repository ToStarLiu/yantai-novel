package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"

	"novel/internal/job"
)

// CandidateReference freezes one reviewed candidate as the target of a new
// author request. It carries no private author preferences and no duplicate prose.
type CandidateReference struct {
	RunID                string `json:"runId"`
	SourceRevisionID     string `json:"sourceRevisionId"`
	TextHash             string `json:"textHash"`
	ParentCandidateRunID string `json:"parentCandidateRunId,omitempty"`
	TotalChars           int    `json:"totalChars"`
}

func candidateReference(run Run) *CandidateReference {
	return &CandidateReference{RunID: run.ID, SourceRevisionID: run.Input.RevisionID, TextHash: job.Hash([]byte(run.Output.Text)), ParentCandidateRunID: run.Input.CandidateRunID, TotalChars: len([]rune(run.Output.Text))}
}

// Candidates are draft suggestions, not original evidence. A collaborator can
// review shared output, but cannot continue another author's private context.
func loadCopilotCandidate(ctx context.Context, db snapshotDB, pid, cid, rid, owner, id string, activeTurn ...string) (Run, error) {
	if id == "" || len(id) > 128 || owner == "" {
		return Run{}, fmt.Errorf("需要有效的候选标识")
	}
	run, err := scanRun(db.QueryRow(ctx, "SELECT "+runColumns+" FROM wb_runs WHERE id=$1 FOR SHARE", id))
	if err != nil {
		return run, err
	}
	if run.ProjectID != pid || run.ChapterID != cid {
		return Run{}, ErrNotFound
	}
	if run.Type != "draft" && run.Type != "revision" && run.Type != "rewrite" {
		return Run{}, fmt.Errorf("只能继续打磨正文候选")
	}
	if run.Status != "Succeeded" || run.Stale || strings.TrimSpace(run.Output.Text) == "" || run.Input.RevisionID != rid {
		return Run{}, &Conflict{Message: "候选所依据的正文已变化，请基于当前正文继续讨论"}
	}
	var creator string
	if err = db.QueryRow(ctx, "SELECT COALESCE(input->>'ownerId','') FROM wb_runs WHERE id=$1", id).Scan(&creator); err != nil {
		return Run{}, err
	}
	if creator != "" && creator != owner {
		return Run{}, ErrForbidden
	}
	if run.Input.AuthorMemory != nil && run.Input.AuthorMemory.OwnerID != owner {
		return Run{}, ErrForbidden
	}
	if strings.HasPrefix(run.IntentID, "copilot-artifact:") {
		parts := strings.SplitN(run.IntentID, ":", 4)
		if len(parts) != 4 {
			return Run{}, &Conflict{Message: "候选对话来源无效"}
		}
		var sourceOwner, sourceStatus string
		var stale bool
		if err = db.QueryRow(ctx, "SELECT owner_id,status,stale FROM wb_copilot_turns WHERE id=$1 FOR SHARE", parts[1]).Scan(&sourceOwner, &sourceStatus, &stale); err != nil {
			if err == pgx.ErrNoRows && len(activeTurn) > 0 && activeTurn[0] == parts[1] {
				sourceOwner, sourceStatus = owner, "Running"
			} else {
				return Run{}, noRows(err)
			}
		}
		if sourceOwner != owner {
			return Run{}, ErrForbidden
		}
		sameActive := len(activeTurn) > 0 && activeTurn[0] != "" && activeTurn[0] == parts[1] && sourceStatus == "Running"
		if (sourceStatus != "Succeeded" && !sameActive) || stale {
			return Run{}, &Conflict{Message: "候选所在的对话未成功完成或已经失效"}
		}
	}
	valid, err := snapshotCurrent(ctx, db, run)
	if err != nil {
		return Run{}, err
	}
	if !valid {
		return Run{}, &Conflict{Message: "候选所依据的章纲、设定、写作指南或作者记忆已变化"}
	}
	if run.Type == "rewrite" {
		source, err := scanRevision(db.QueryRow(ctx, "SELECT "+revisionColumns+" FROM wb_revisions WHERE id=$1", rid))
		if err != nil {
			return Run{}, err
		}
		if source.ChapterID != cid || run.Input.Selection == "" || strings.Count(source.PlainText, run.Input.Selection) != 1 {
			return Run{}, &Conflict{Message: "选段候选缺少唯一原稿锚点，不能作为整章继续改稿"}
		}
		run.Output.Text = strings.Replace(source.PlainText, run.Input.Selection, run.Output.Text, 1)
	}
	return run, nil
}

func copilotCandidateCurrent(ctx context.Context, db snapshotDB, pid, cid, rid, owner string, ref *CandidateReference) (bool, error) {
	if ref == nil {
		return true, nil
	}
	run, err := loadCopilotCandidate(ctx, db, pid, cid, rid, owner, ref.RunID)
	if err != nil {
		if IsConflict(err) || err == ErrNotFound || err == ErrForbidden {
			return false, nil
		}
		return false, err
	}
	got := candidateReference(run)
	return *got == *ref, nil
}

func candidateReadTool() CopilotToolDefinition {
	return CopilotToolDefinition{Name: "read_revision_candidate", Description: "分页读本轮绑定候选或其选段，offset=nextOffset。候选不是原稿证据。", Parameters: json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer","minimum":0},"maxChars":{"type":"integer","minimum":200,"maximum":4000},"selection":{"type":"boolean"}},"additionalProperties":false}`)}
}

func (s *Service) readCopilotCandidate(ctx context.Context, scope CopilotToolContext, raw json.RawMessage) (CopilotToolResult, error) {
	if scope.Candidate == nil {
		return CopilotToolResult{}, fmt.Errorf("本轮未绑定需要打磨的候选")
	}
	var args struct {
		Offset    int  `json:"offset"`
		MaxChars  int  `json:"maxChars"`
		Selection bool `json:"selection"`
	}
	if err := copilotDecode(raw, &args); err != nil {
		return CopilotToolResult{}, err
	}
	if args.MaxChars == 0 {
		args.MaxChars = 2000
	}
	if args.Offset < 0 || args.MaxChars < 200 || args.MaxChars > 4000 {
		return CopilotToolResult{}, fmt.Errorf("候选续读位置或长度无效")
	}
	run, err := loadCopilotCandidate(ctx, s.DB, scope.ProjectID, scope.ChapterID, scope.RevisionID, scope.Principal.ID, scope.Candidate.RunID)
	if err != nil {
		return CopilotToolResult{}, err
	}
	text, source := run.Output.Text, "revision-candidate"
	if args.Selection {
		if scope.CandidateSelection == "" {
			return CopilotToolResult{}, fmt.Errorf("本轮没有绑定候选选段")
		}
		text, source = scope.CandidateSelection, "revision-candidate-selection"
	}
	runes := []rune(text)
	if args.Offset > len(runes) {
		return CopilotToolResult{}, fmt.Errorf("候选续读超出范围")
	}
	end := min(len(runes), args.Offset+args.MaxChars)
	return CopilotToolResult{Content: map[string]any{"source": source, "runId": run.ID, "parentCandidateRunId": run.Input.CandidateRunID, "sourceRevisionId": run.Input.RevisionID, "hash": job.Hash([]byte(text)), "candidateHash": scope.Candidate.TextHash, "text": string(runes[args.Offset:end]), "offset": args.Offset, "nextOffset": end, "totalChars": len(runes), "truncated": args.Offset > 0 || end < len(runes), "selectionHash": job.Hash([]byte(scope.CandidateSelection)), "selectionChars": len([]rune(scope.CandidateSelection)), "authority": "这是未采用的候选，不是作者已保存正文或确认事实；分页改稿只替换本轮精确选段，其他候选修改仍保留。"}}, nil
}

func candidateCopilotContext(ref *CandidateReference, selection string) any {
	return map[string]any{"runId": ref.RunID, "sourceRevisionId": ref.SourceRevisionID, "hash": ref.TextHash, "parentCandidateRunId": ref.ParentCandidateRunID, "totalChars": ref.TotalChars, "selectionChars": len([]rune(selection)), "selectionHash": job.Hash([]byte(selection)), "readTool": "read_revision_candidate"}
}

const candidateAuthority = "\n本轮绑定candidate是作者要求继续打磨的唯一候选。先用read_revision_candidate分页读取相关内容；候选是未采用建议而非事实。candidateSelection存在时，selection模式替换该候选的完整选段，其他改动保留；否则chapter模式提交完整新候选。新候选保留父候选关系，sourceRevisionId始终指已保存原稿，作者明确采用才写正文。"

// Extra candidate capabilities and their schemas exist only when an author has
// explicitly selected a parent; ordinary startup retains its previous budget.
func candidateIterationTools(base []CopilotToolDefinition) []CopilotToolDefinition {
	tools := append([]CopilotToolDefinition(nil), base...)
	for i := range tools {
		if tools[i].Name == "show_diff" {
			tools[i].Description = "比较候选与原稿；compareTo=parent只读比较上一稿，不改正文。"
			tools[i].Parameters = json.RawMessage(`{"type":"object","properties":{"runId":{"type":"string"},"text":{"type":"string","maxLength":60000},"compareTo":{"type":"string","enum":["source","parent"]}},"additionalProperties":false}`)
		}
	}
	return append(tools, candidateReadTool())
}
