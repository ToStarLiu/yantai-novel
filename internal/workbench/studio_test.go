package workbench

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestStudioBoardVersionCausalValidationAndBranches(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	person, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "character", Name: "林川"})
	if err != nil {
		t.Fatal(err)
	}
	board := StudioBoard{Nodes: []StudioNode{{ID: "setup", Type: "event", Label: "发现线索", X: 10, Y: 20, ChapterID: c.ID, ObservedBy: []string{person.ID}}, {ID: "reveal", Type: "plot", Label: "揭示真相", X: 200, Y: 80, PrerequisiteIDs: []string{"setup"}}}, Edges: []StudioEdge{{ID: "cause", Source: "setup", Target: "reveal", Kind: "cause"}}, Conditions: []StudioCondition{{ID: "condition", NodeID: "reveal", RequiresNodeIDs: []string{"setup"}, Expression: "发现线索后才揭示"}}, Endings: []StudioEnding{{ID: "ending", NodeID: "reveal", Title: "真相结局", ConditionIDs: []string{"condition"}}}}
	saved, err := s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{Data: board})
	if err != nil || saved.Version != 1 {
		t.Fatalf("save %v %#v", err, saved)
	}
	evaluation, err := s.EvaluateStudioBoard(ctx, p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.AvailableNodeIDs) != 1 || evaluation.AvailableNodeIDs[0] != "setup" || len(evaluation.Unmet["reveal"]) == 0 {
		t.Fatalf("prerequisite not enforced %#v", evaluation)
	}
	evaluation, err = s.EvaluateStudioBoard(ctx, p.ID, []string{"setup", "reveal"})
	if err != nil || len(evaluation.MatchedEndingIDs) != 1 {
		t.Fatalf("ending %v %#v", err, evaluation)
	}
	withoutCause, err := s.EvaluateStudioBoard(ctx, p.ID, []string{"reveal"})
	if err != nil || len(withoutCause.MatchedEndingIDs) != 0 {
		t.Fatal("ending bypassed causal prerequisites")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicts := 0, 0
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			copy := board
			copy.Nodes = append([]StudioNode{}, board.Nodes...)
			copy.Nodes[0].X = float64(100 + i)
			_, e := s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{ExpectedVersion: 1, Data: copy})
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				ok++
			} else if IsConflict(e) {
				conflicts++
			} else {
				t.Error(e)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if ok != 1 || conflicts != 1 {
		t.Fatalf("lost drag update ok=%d conflict=%d", ok, conflicts)
	}
	invalid := board
	invalid.Nodes = append([]StudioNode{}, board.Nodes...)
	invalid.Nodes[0].PrerequisiteIDs = []string{"reveal"}
	if _, err = s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{ExpectedVersion: 2, Data: invalid}); err == nil {
		t.Fatal("causal cycle accepted")
	}
	_, foreign := setupCore(t, s)
	invalid = board
	invalid.Nodes = append([]StudioNode{}, board.Nodes...)
	invalid.Nodes[0].ChapterID = foreign.ID
	if _, err = s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{ExpectedVersion: 2, Data: invalid}); err == nil {
		t.Fatal("foreign chapter accepted")
	}
}

func TestStudioPlanningScenesArcsPersistWithScopeValidation(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	person, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "character", Name: "沈月"})
	if err != nil {
		t.Fatal(err)
	}
	brief := StoryBrief{Audience: "中文悬疑读者", Theme: "信任的代价", SellingPoint: "不可靠证人", CoreConflict: "守住秘密还是救人", ProtagonistDesire: "找到失踪信使", FailureCost: "同伴被捕", EndingDirection: "揭示误信", TargetWords: 100000, TargetChapters: 30, WorldRules: []string{"禁止瞬间移动"}, Volumes: []StoryVolume{{ID: "v1", Title: "第一卷", Goal: "获得铜印", ChapterCount: 10}}}
	if _, err = s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{Data: brief}); err != nil {
		t.Fatal(err)
	}
	scenes := SceneCollection{Scenes: []StudioScene{{ID: "scene1", ChapterID: c.ID, Order: 1, Title: "入磨坊", Goal: "避雨", Conflict: "铜印来历不明", ObservedBy: []string{person.ID}, Handoff: SceneHandoff{EndLocation: "磨坊", PresentCharacters: []string{person.ID}, OpenActions: []string{"准备开箱"}, ForbiddenChanges: []string{"左臂不能无故痊愈"}}}, {ID: "scene2", ChapterID: c.ID, Order: 2, Title: "启箱", Causes: []string{"scene1"}}}}
	if _, err = s.SaveStudioScenes(ctx, p.ID, StudioWrite[SceneCollection]{Data: scenes}); err != nil {
		t.Fatal(err)
	}
	arcs := CharacterArcs{Characters: []CharacterArc{{EntityID: person.ID, Arc: "从隐瞒走向坦白", Emotion: "警惕", Goal: "保护同伴", BeliefImportance: .8, RecencyWeight: .2, ForgetAfterChapters: 10}}}
	if _, err = s.SaveStudioArcs(ctx, p.ID, StudioWrite[CharacterArcs]{Data: arcs}); err != nil {
		t.Fatal(err)
	}
	view, err := s.Studio(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Brief.Data.CoreConflict != brief.CoreConflict || view.Scenes.Data.Scenes[0].Handoff.ForbiddenChanges[0] != "左臂不能无故痊愈" || view.Arcs.Data.Characters[0].Emotion != "警惕" {
		t.Fatal("structured studio data not preserved")
	}
	scenes.Scenes[0].Causes = []string{"scene2"}
	if _, err = s.SaveStudioScenes(ctx, p.ID, StudioWrite[SceneCollection]{ExpectedVersion: 1, Data: scenes}); err == nil {
		t.Fatal("scene cause cycle accepted")
	}
	arcs.Characters[0].EntityID = "foreign"
	if _, err = s.SaveStudioArcs(ctx, p.ID, StudioWrite[CharacterArcs]{ExpectedVersion: 1, Data: arcs}); err == nil {
		t.Fatal("foreign arc accepted")
	}
}

func TestStudioReferenceLibraryIsIsolatedAndAnalyzed(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	ctx := context.Background()
	text := "# 雪夜\n霜狼密约藏在塔楼。\n她说：“我只知道一半。”\n# 次日\n另一段参考材料。"
	ref, err := s.AddStudioReference(ctx, p.ID, "参考结构", text, "作者授权参考")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Analysis.WordCount == 0 || ref.Analysis.ParagraphCount != 5 || len(ref.Analysis.Structure) != 2 || ref.Analysis.DialogueRatio <= 0 || ref.Analysis.WritingProfile == "" {
		t.Fatalf("analysis %#v", ref.Analysis)
	}
	hits, err := s.SearchStudioReferences(ctx, p.ID, "霜狼密约")
	if err != nil || len(hits) != 1 || !strings.Contains(hits[0].Quote, "霜狼密约") {
		t.Fatalf("reference search %v %#v", err, hits)
	}
	main, err := s.Search(ctx, Query{ProjectID: p.ID, Text: "霜狼密约", Purpose: "AuthorReview"})
	if err != nil || len(main) != 0 {
		t.Fatal("reference text leaked into canonical search")
	}
	other, err := s.CreateProject(ctx, "另一作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.SearchStudioReferences(ctx, other.ID, "霜狼密约")
	if err != nil || len(foreign) != 0 {
		t.Fatal("reference search crossed project")
	}
	if err = s.DeleteStudioReference(ctx, other.ID, ref.ID); err != ErrNotFound {
		t.Fatal("foreign reference delete accepted")
	}
	if err = s.DeleteStudioReference(ctx, p.ID, ref.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := s.StudioReferences(ctx, p.ID)
	if err != nil || len(remaining) != 0 {
		t.Fatal("deleted reference remained visible")
	}
	var history int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_studio_history WHERE project_id=$1 AND id=$2", p.ID, ref.ID).Scan(&history); err != nil || history != 0 {
		t.Fatal("reference deletion retained private text in history")
	}
}

func TestStudioSandboxPreviewConfirmRollbackNeverWritesCanonical(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter := f.chapter(t, "第一章", "林川在北岸磨坊。左臂疼痛。")
	state := f.apply(t, f.proposal(t, chapter, []MemoryChange{f.change(t, chapter, "Location", f.lin.ID, "location", "北岸磨坊", "", "True", "林川在北岸磨坊。"), f.change(t, chapter, "Injury", f.lin.ID, "injury", "左臂疼痛", "", "True", "左臂疼痛。")}))
	box, err := f.s.CreateStudioSandbox(ctx, f.p.ID, SandboxCreate{Title: "势力代价演练", EntityIDs: []string{f.lin.ID}, Steps: 2, Seed: 17, Parameters: SandboxParameters{InitialResources: 20, InitialTension: 80, ResourceDelta: 2, TensionDelta: -2, EventLabel: "夜间调停"}})
	if err != nil {
		t.Fatal(err)
	}
	if box.State.Turn != 0 || box.Candidate == nil || len(box.Candidate.Events) != 2 || len(box.Candidate.Diff) == 0 || box.State.Actors[0].Location != "北岸磨坊" || len(box.InitialEvidence) != 2 {
		t.Fatalf("sandbox %#v", box)
	}
	originalCheckpoint := box.History[0].ID
	confirmed, err := f.s.StudioSandboxAction(ctx, f.p.ID, box.ID, "confirm", box.Version, 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.State.Turn != 2 || confirmed.Candidate != nil || len(confirmed.History) != 2 {
		t.Fatal("confirmation did not commit sandbox state")
	}
	if _, err = f.s.StudioSandboxAction(ctx, f.p.ID, box.ID, "confirm", 1, 0, 0, ""); !IsConflict(err) {
		t.Fatal("stale sandbox confirmation accepted")
	}
	rolled, err := f.s.StudioSandboxAction(ctx, f.p.ID, box.ID, "rollback", confirmed.Version, 0, 0, originalCheckpoint)
	if err != nil || rolled.State.Turn != 0 || len(rolled.History) != 3 {
		t.Fatalf("rollback %v %#v", err, rolled)
	}
	frozen, err := f.s.StudioSandboxAction(ctx, f.p.ID, box.ID, "freeze", rolled.Version, 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.StudioSandboxAction(ctx, f.p.ID, box.ID, "simulate", frozen.Version, 1, 1, ""); !IsConflict(err) {
		t.Fatal("frozen sandbox simulated")
	}
	after, err := f.s.State(ctx, f.p.ID)
	if err != nil || after.StateVersionID != state.StateVersionID || len(after.Facts) != 2 {
		t.Fatal("sandbox changed canonical facts")
	}
	current, err := f.s.GetChapter(ctx, chapter.ID)
	if err != nil || current.CurrentRevisionID != chapter.CurrentRevisionID {
		t.Fatal("sandbox changed manuscript")
	}
}

func TestStudioWikiBeliefProjectionAndCommentsKeepAuthority(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "一", "林川误以为铜印是商队凭牌。铜印留在内袋。")
	f.apply(t, f.proposal(t, c, []MemoryChange{f.change(t, c, "Knowledge", f.seal.ID, "origin", "商队凭牌", f.lin.ID, "False", "林川误以为铜印是商队凭牌。")}))
	if _, err := f.s.CreateChapter(ctx, f.p.ID, "二", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.SaveStudioArcs(ctx, f.p.ID, StudioWrite[CharacterArcs]{Data: CharacterArcs{Characters: []CharacterArc{{EntityID: f.lin.ID, Arc: "误解到真相", BeliefImportance: .9, RecencyWeight: .5, ForgetAfterChapters: 1}}}}); err != nil {
		t.Fatal(err)
	}
	pages, err := f.s.StudioWiki(ctx, f.p.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, page := range pages {
		if page.EntityID == f.lin.ID {
			found = len(page.Beliefs) == 1 && page.Beliefs[0].ForgottenProjection && page.Beliefs[0].Fact.TruthStatus == "False"
		}
	}
	if !found {
		t.Fatal("belief importance/forgetting projection missing")
	}
	state, err := f.s.State(ctx, f.p.ID)
	if err != nil || len(state.Facts) != 1 {
		t.Fatal("forget projection deleted authoritative memory")
	}
	comment, err := f.s.AddStudioComment(ctx, f.p.ID, StudioCommentInput{RevisionID: c.CurrentRevisionID, Quote: "铜印留在内袋。", Text: "此处保留物品线索"})
	if err != nil {
		t.Fatal(err)
	}
	if comment.AnchorStatus != "Located" {
		t.Fatal("valid anchor not located")
	}
	updated, err := f.s.UpdateStudioComment(ctx, f.p.ID, comment.ID, comment.Version, "已补后续回收", true)
	if err != nil || !updated.Resolved || updated.Version != 2 {
		t.Fatalf("comment update %v %#v", err, updated)
	}
	if _, err = f.s.UpdateStudioComment(ctx, f.p.ID, comment.ID, 1, "lost update", false); !IsConflict(err) {
		t.Fatal("stale comment update accepted")
	}
	if _, err = f.s.AddStudioComment(ctx, f.p.ID, StudioCommentInput{RevisionID: c.CurrentRevisionID, Quote: "不存在的引文", Text: "伪造"}); err == nil {
		t.Fatal("fabricated comment quote accepted")
	}
}

func TestStudioForkRemapsEverythingAndKeepsBranchesIndependent(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	person, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "character", Name: "林川"})
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := saveCore(t, s, current, "铜印在林川手中。")
	_, err = s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{Data: StudioBoard{Nodes: []StudioNode{{ID: "scene-node", Type: "scene", Label: "抉择", ChapterID: c.ID, EntityID: person.ID, ObservedBy: []string{person.ID}}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveStudioScenes(ctx, p.ID, StudioWrite[SceneCollection]{Data: SceneCollection{Scenes: []StudioScene{{ID: "scene-a", ChapterID: c.ID, Order: 1, Title: "选择", ObservedBy: []string{person.ID}}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddStudioComment(ctx, p.ID, StudioCommentInput{RevisionID: r.ID, Quote: "铜印", Text: "分支前注释"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.ForkStudioProject(ctx, p.ID, "另一条道路", detail.Project.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.Studio(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := s.Studio(ctx, fork.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if copy.Board.Data.Nodes[0].ID == original.Board.Data.Nodes[0].ID || copy.Board.Data.Nodes[0].ChapterID == c.ID || copy.Board.Data.Nodes[0].EntityID == person.ID || copy.Scenes.Data.Scenes[0].ID == "scene-a" || copy.Comments[0].RevisionID == r.ID {
		t.Fatal("branch retained original object identities")
	}
	if copy.Board.Data.Nodes[0].ObservedBy[0] != fork.Settings[0].ID || copy.Comments[0].Quote != "铜印" {
		t.Fatal("branch references not mapped")
	}
	copy.Board.Data.Nodes[0].Content = "只在分支中变化"
	if _, err = s.SaveStudioBoard(ctx, fork.Project.ID, StudioWrite[StudioBoard]{ExpectedVersion: copy.Board.Version, Data: copy.Board.Data}); err != nil {
		t.Fatal(err)
	}
	again, err := s.Studio(ctx, p.ID)
	if err != nil || again.Board.Data.Nodes[0].Content != "" {
		t.Fatal("branch edits changed source")
	}
	links, err := s.StudioBranches(ctx, p.ID)
	if err != nil || len(links) != 1 || links[0].TargetProjectID != fork.Project.ID {
		t.Fatal("branch relation missing")
	}
	archive, err := s.ExportProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err = json.Unmarshal(archive.Studio, &data); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), "sourceProjectId") {
		t.Fatal("branch link recursively included in package")
	}
}

func TestStudioArchiveRejectsForgedObserverCycleAndCommentSource(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	person, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "character", Name: "林川"})
	if err != nil {
		t.Fatal(err)
	}
	board := StudioBoard{Nodes: []StudioNode{{ID: "a", Type: "event", Label: "起因", ObservedBy: []string{person.ID}}, {ID: "b", Type: "event", Label: "结果", PrerequisiteIDs: []string{"a"}}}}
	if _, err = s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{Data: board}); err != nil {
		t.Fatal(err)
	}
	archive, err := s.ExportProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*StudioBoard){func(b *StudioBoard) { b.Nodes[0].ObservedBy = []string{"foreign-character"} }, func(b *StudioBoard) { b.Nodes[0].PrerequisiteIDs = []string{"b"} }, func(b *StudioBoard) { b.Nodes[0].ChapterID = "foreign-chapter" }} {
		raw, _ := json.Marshal(archive)
		var copied ProjectPackage
		_ = json.Unmarshal(raw, &copied)
		var studio StudioArchive
		_ = json.Unmarshal(copied.Studio, &studio)
		for i, doc := range studio.Documents {
			if doc.Kind == "board" {
				var b StudioBoard
				_ = json.Unmarshal(doc.Data, &b)
				mutate(&b)
				studio.Documents[i].Data, _ = json.Marshal(b)
			}
		}
		copied.Studio, _ = json.Marshal(studio)
		if _, err = s.ImportProject(ctx, copied); err == nil {
			t.Fatal("forged studio archive accepted")
		}
	}
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Chapters) != 1 || detail.Chapters[0].ID != c.ID {
		t.Fatal("invalid import mutated source")
	}
	projects, err := s.ListProjects(ctx)
	if err != nil || len(projects) != 1 {
		t.Fatal("failed studio import left partial project")
	}
}

func TestStudioPromptOnlyPlanningAndVersionChangesInvalidate(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	ctx := context.Background()
	before, err := s.StudioPromptContext(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Versions) != 3 || before.Versions["brief"] != 0 || string(before.Data) != "{}" {
		t.Fatalf("empty planning %#v", before)
	}
	if _, err = s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{Data: StoryBrief{CoreConflict: "救人还是守密", Audience: "悬疑读者"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddStudioReference(ctx, p.ID, "私有参考", "参考密词不可进模型事实上下文", ""); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.StudioPromptContext(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Versions["brief"] != 1 || !strings.Contains(string(snapshot.Data), "救人还是守密") || strings.Contains(string(snapshot.Data), "参考密词") {
		t.Fatalf("planning projection %s", snapshot.Data)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := ValidateStudioPromptTx(ctx, tx, p.ID, snapshot.Versions)
	_ = tx.Rollback(ctx)
	if err != nil || !valid {
		t.Fatal("matching snapshot rejected")
	}
	if _, err = s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{ExpectedVersion: 1, Data: StoryBrief{CoreConflict: "新的主冲突"}}); err != nil {
		t.Fatal(err)
	}
	tx, err = s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	valid, err = ValidateStudioPromptTx(ctx, tx, p.ID, snapshot.Versions)
	_ = tx.Rollback(ctx)
	if err != nil || valid {
		t.Fatal("stale Studio prompt accepted")
	}
}

func TestStudioTimelineProjectionAndHistoryRestore(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	ctx := context.Background()
	day1, day5, day10 := 1, 5, 10
	board := StudioBoard{Nodes: []StudioNode{{ID: "early", Type: "event", Label: "早期事件", StoryTime: &day1}, {ID: "late", Type: "event", Label: "后续事件", StoryTime: &day10}, {ID: "limited", Type: "faction", Label: "短期联盟", ValidFrom: &day1, ValidTo: &day5}}, Edges: []StudioEdge{{ID: "edge", Source: "early", Target: "late", Kind: "cause"}}}
	if _, err := s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{Data: board}); err != nil {
		t.Fatal(err)
	}
	at, err := s.StudioBoardAt(ctx, p.ID, 3)
	if err != nil || len(at.Data.Nodes) != 2 || len(at.Data.Edges) != 0 {
		t.Fatalf("timeline projection %v %#v", err, at)
	}
	at, err = s.StudioBoardAt(ctx, p.ID, 10)
	if err != nil || len(at.Data.Nodes) != 2 || len(at.Data.Edges) != 1 {
		t.Fatal("ended relation visible or future event missing")
	}
	board.Nodes[0].X = 400
	if _, err = s.SaveStudioBoard(ctx, p.ID, StudioWrite[StudioBoard]{ExpectedVersion: 1, Data: board}); err != nil {
		t.Fatal(err)
	}
	restored, err := s.RestoreStudioHistory(ctx, p.ID, "board", "main", 2, 1)
	if err != nil || restored.Version != 3 {
		t.Fatalf("restore %v %#v", err, restored)
	}
	view, err := s.Studio(ctx, p.ID)
	if err != nil || view.Board.Data.Nodes[0].X != 0 {
		t.Fatal("board historical content not restored")
	}
	history, err := s.StudioHistory(ctx, p.ID, "board", "main")
	if err != nil || len(history) != 3 {
		t.Fatal("restore deleted history")
	}
}
