package workbench

import (
	"context"
	"testing"
)

func TestProjectDeletionIncludesProductionStepsWithoutTouchingOtherProjects(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	other, err := s.CreateProject(ctx, "保留的另一作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "delete-project-flow", Version: 1, Steps: []WorkflowStep{{ID: "draft", Type: "draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	production, err := s.StartProduction(ctx, c.ProjectID, ProductionRequest{WorkflowID: flow.ID, ChapterIDs: []string{c.ID}, MaxChapters: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceAutomations(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteProject(ctx, c.ProjectID); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_production_steps WHERE production_id=$1", production.ID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("orphan production step", remaining, err)
	}
	if _, err = s.GetProject(ctx, other.ID); err != nil {
		t.Fatal("unrelated project removed", err)
	}
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_workflows WHERE id=$1", flow.ID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatal("global reusable workflow removed", remaining, err)
	}
}
