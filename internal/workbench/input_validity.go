package workbench

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type snapshotDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// snapshotCurrent is shared by submission, dispatch, completion and recovery.
// The shared locks keep this check valid until the surrounding transaction
// commits, so a finalization cannot race a derived result into the active line.
func snapshotCurrent(ctx context.Context, db snapshotDB, run Run) (bool, error) {
	var epoch, blueprint int64
	var state, current, final, projectID string
	var deleted bool
	err := db.QueryRow(ctx, `SELECT p.id,p.epoch,p.state_version_id,c.blueprint_version,c.current_revision_id,c.final_revision_id,c.deleted FROM wb_chapters c JOIN wb_projects p ON p.id=c.project_id WHERE c.id=$1 FOR SHARE OF p,c`, run.ChapterID).Scan(&projectID, &epoch, &state, &blueprint, &current, &final, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if run.Input.ProjectID != "" && projectID != run.Input.ProjectID {
		return false, nil
	}
	if deleted || epoch != run.Input.Epoch || state != run.Input.StateVersionID || blueprint != run.Input.BlueprintVersion {
		return false, nil
	}
	revision := current
	if run.Type == "memory-extraction" {
		revision = final
	}
	if revision != run.Input.RevisionID {
		return false, nil
	}
	// Style preferences cannot invalidate evidence derived from finalized prose.
	if run.Type != "memory-extraction" {
		if valid, err := authorMemoryBaselineCurrent(ctx, db, run.Input.AuthorMemory); err != nil || !valid {
			return valid, err
		}
		if valid, err := writingSkillSnapshotCurrent(ctx, db, projectID, run.Input.WritingSkills); err != nil || !valid {
			return valid, err
		}
	}
	if len(run.Input.StudioVersions) > 0 {
		if tx, ok := db.(pgx.Tx); ok {
			valid, err := ValidateStudioPromptTx(ctx, tx, projectID, run.Input.StudioVersions)
			if err != nil || !valid {
				return valid, err
			}
		} else {
			if len(run.Input.StudioVersions) != 3 {
				return false, nil
			}
			for _, kind := range []string{"brief", "scenes", "arcs"} {
				version, ok := run.Input.StudioVersions[kind]
				if !ok {
					return false, nil
				}
				var current int64
				e := db.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM wb_studio_documents WHERE project_id=$1 AND kind=$2 AND id='main'),0)", projectID, kind).Scan(&current)
				if e != nil {
					return false, e
				}
				if current != version {
					return false, nil
				}
			}
		}
	}
	if len(run.Input.CanvasNodeIDs) > 0 {
		if tx, ok := db.(pgx.Tx); ok {
			return ValidateCanvasSnapshotTx(ctx, tx, projectID, run.Input.CanvasBoardVersion)
		}
		var current int64
		err := db.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM wb_studio_documents WHERE project_id=$1 AND kind='board' AND id='main'),0)", projectID).Scan(&current)
		return err == nil && current == run.Input.CanvasBoardVersion, err
	}
	return true, nil
}
