package workbench

import (
	"context"
	"encoding/json"
	"novel/internal/job"
	"strings"
	"testing"
	"time"
)

func TestAdvancedProjectArchivePreservesDocumentsResearchAndReadOnlyActivity(t *testing.T) {
	s := newCore(t)
	featureProvider(s)
	p, c := setupCore(t, s)
	ctx := context.Background()
	r := saveCore(t, s, c, "林川收到铜印，在雨夜推开磨坊的门。")
	actor, e := s.SaveSetting(ctx, p.ID, Setting{Kind: "character", Name: "林川"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveSetting(ctx, p.ID, Setting{Kind: "style", Name: "采用作者声音", Content: "短句与动作"}); e != nil {
		t.Fatal(e)
	}
	board, e := s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{Data: StudioBoard{Nodes: []StudioNode{{ID: "n1", Type: "scene", Label: "雨夜", ChapterID: c.ID, ObservedBy: []string{actor.ID}}}}})
	if e != nil {
		t.Fatal(e)
	}
	board.Data.Nodes[0].X = 55
	if _, e = s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{ExpectedVersion: 1, Data: board.Data}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{Data: StoryBrief{Theme: "代价", Volumes: []StoryVolume{{ID: "vol1", Title: "夜雨"}}}}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveStudioScenes(ctx, p.ID, StudioWrite[SceneCollection]{Data: SceneCollection{Scenes: []StudioScene{{ID: "scene1", ChapterID: c.ID, Title: "雨夜开门", Order: 1, NodeIDs: []string{"n1"}, BoardVersion: 2, ObservedBy: []string{actor.ID}}}}}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveStudioArcs(ctx, p.ID, StudioWrite[CharacterArcs]{Data: CharacterArcs{Characters: []CharacterArc{{EntityID: actor.ID, Arc: "承担代价", Emotion: "紧张"}}}}); e != nil {
		t.Fatal(e)
	}
	reference, e := s.AddStudioReference(ctx, p.ID, "原创来源", "# 第一幕\n门外是雨声和未知的脚步。", "作者输入")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddStudioComment(ctx, p.ID, StudioCommentInput{RevisionID: r.ID, Quote: "收到铜印", Text: "注意物品连续性"}); e != nil {
		t.Fatal(e)
	}
	box, e := s.CreateStudioSandbox(ctx, p.ID, SandboxCreate{Title: "探索", EntityIDs: []string{actor.ID}, Steps: 2, Parameters: SandboxParameters{InitialResources: 10, ResourceDelta: -2, TensionDelta: 1}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.StudioSandboxAction(ctx, p.ID, box.ID, "confirm", box.Version, 0, 0, ""); e != nil {
		t.Fatal(e)
	}
	sources, _ := json.Marshal([]ResearchSource{{URL: "https://example.com/original", FetchedAt: time.Now().UTC(), Title: "原创来源", Text: "门外是雨声和未知的脚步。", Excerpt: "门外是雨声", ReferenceID: reference.ID}, {URL: "https://example.com/missing", Error: "HTTP404"}})
	if _, e = s.DB.Exec(ctx, "INSERT INTO wb_research_reports(id,project_id,topic,status,sources) VALUES($1,$2,'结构研究','Partial',$3)", job.NewID(), p.ID, sources); e != nil {
		t.Fatal(e)
	}
	run, e := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: "unadopted-output", TaskType: "analysis"})
	if e != nil {
		t.Fatal(e)
	}
	run = featureFinish(t, s, run)
	workflow, e := s.SaveWorkflow(ctx, WorkflowDefinition{Name: "归档流程", Version: 1, Steps: []WorkflowStep{{ID: "one", Type: "analysis"}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.StartProduction(ctx, p.ID, ProductionRequest{WorkflowID: workflow.ID, ChapterIDs: []string{c.ID}, ManualGates: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.StartCandidates(ctx, c.ID, CandidateRequest{Count: 2}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, "INSERT INTO wb_media(id,project_id,chapter_id,revision_id,kind,status,input,content,mime) VALUES($1,$2,$3,$4,'cover','Succeeded','{}',$5,'image/png')", job.NewID(), p.ID, c.ID, r.ID, []byte("media-bytes-not-in-json")); e != nil {
		t.Fatal(e)
	}
	pkg, e := s.ExportProject(ctx, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(pkg)
	if strings.Contains(string(raw), "media-bytes-not-in-json") {
		t.Fatal("binary leaked into project package")
	}
	var st StudioArchive
	_ = json.Unmarshal(pkg.Studio, &st)
	if len(st.Documents) != 7 || len(st.History) < 9 || len(pkg.Research) != 1 {
		t.Fatalf("incomplete documents %d/%d/%d", len(st.Documents), len(st.History), len(pkg.Research))
	}
	var activity ProjectActivityArchive
	if e = json.Unmarshal(pkg.Activity, &activity); e != nil || len(activity.Runs) != 1 || len(activity.Productions) != 1 || len(activity.Candidates) != 1 || len(activity.Media) != 1 {
		t.Fatal("activity missing", e)
	}
	imported, e := s.ImportProject(ctx, pkg)
	if e != nil {
		t.Fatal(e)
	}
	newID := imported.Project.ID
	view, e := s.Studio(ctx, newID)
	if e != nil {
		t.Fatal(e)
	}
	if view.Board.Version != 2 || len(view.Sandboxes) != 1 || len(view.Sandboxes[0].History) < 2 || len(view.Comments) != 1 || len(view.Scenes.Data.Scenes[0].NodeIDs) != 1 {
		t.Fatal("studio document loss", e)
	}
	history, e := s.StudioHistory(ctx, newID, "board", "main")
	if e != nil || len(history) != 2 {
		t.Fatal("history loss", e, len(history))
	}
	reports, e := s.ListResearch(ctx, newID)
	if e != nil || len(reports) != 1 || len(reports[0].Sources) != 2 || reports[0].Sources[0].ReferenceID == reference.ID {
		t.Fatal("research provenance loss", e)
	}
	arch, e := s.ProjectArchive(ctx, newID)
	if e != nil || !arch.Imported || len(arch.Activity.Prior) != 1 {
		t.Fatal("read-only archive missing", e)
	}
	for _, table := range []string{"wb_runs", "wb_productions", "wb_candidate_groups", "wb_media"} {
		var count int
		if e = s.DB.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE project_id=$1", newID).Scan(&count); e != nil || count != 0 {
			t.Fatalf("import revived queue %s count%d %v", table, count, e)
		}
	}
	again, e := s.ExportProject(ctx, newID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImportProject(ctx, again); e != nil {
		t.Fatal("second roundtrip failed", e)
	}
}

func TestAdvancedArchiveForeignResearchAndActivityRejectedAtomically(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	ctx := context.Background()
	pkg, e := s.ExportProject(ctx, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, mutation := range []func(*ProjectPackage){func(v *ProjectPackage) {
		v.Research = []ResearchReport{{ID: "report", ProjectID: p.ID, Status: "Success", Sources: []ResearchSource{{ReferenceID: "foreign-reference"}}}}
	}, func(v *ProjectPackage) {
		a := ProjectActivityArchive{SchemaVersion: 1, ReadOnly: true, SourceProjectID: "foreign-project"}
		v.Activity, _ = json.Marshal(a)
	}} {
		raw, _ := json.Marshal(pkg)
		var copy ProjectPackage
		_ = json.Unmarshal(raw, &copy)
		mutation(&copy)
		if _, e = s.ImportProject(ctx, copy); e == nil {
			t.Fatal("forged archive accepted")
		}
	}
	projects, e := s.ListProjects(ctx)
	if e != nil || len(projects) != 1 {
		t.Fatal("bad archive left partial project", e, len(projects))
	}
}
