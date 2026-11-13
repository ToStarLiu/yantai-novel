package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"novel/internal/job"
)

func copilotAdmin() context.Context {
	return WithPrincipal(context.Background(), Principal{ID: "admin", Username: "author", Admin: true})
}
func copilotScope(t *testing.T, s *Service, c Chapter, selection string) CopilotToolContext {
	t.Helper()
	scope, err := s.NewCopilotToolContext(copilotAdmin(), job.NewID(), c.ProjectID, c.ID, c.CurrentRevisionID, selection)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
func copilotArgs(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }

func TestCopilotToolsCandidateDiffDoesNotWriteUntilAuthorAdopts(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	r, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("林川左臂受伤。\n\n沈月说得很直接。\n\n铜印仍由林川持有。")})
	if err != nil {
		t.Fatal(err)
	}
	c.CurrentRevisionID = r.ID
	scope := copilotScope(t, s, c, "沈月说得很直接。")
	scope.CallID = "candidate-call"
	args := copilotArgs(map[string]string{"text": "沈月避开了他的目光，只说还需要等一等。", "mode": "selection", "title": "保留秘密"})
	result, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", args)
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact == nil || result.Artifact.Kind != "revision-candidate" || result.Artifact.RunID == "" || result.Artifact.SourceHash != r.Hash {
		t.Fatalf("missing source artifact %#v", result)
	}
	if !strings.Contains(result.Artifact.Text, "林川左臂受伤。") || !strings.Contains(result.Artifact.Text, "铜印仍由林川持有。") || strings.Contains(result.Artifact.Text, "说得很直接") {
		t.Fatal("selection replacement changed other text")
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil || current.CurrentRevisionID != r.ID {
		t.Fatal("tool replaced manuscript", err)
	}
	run, err := s.GetRun(ctx, result.Artifact.RunID)
	if err != nil || run.Status != "Succeeded" || run.Type != "revision" || run.Input.RevisionID != r.ID || run.CostStatus != "IncludedInCopilotAttempt" {
		t.Fatal("candidate not durable", err)
	}
	var attempts int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_run_attempts WHERE run_id=$1", run.ID).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("candidate generated extra external attempt", err)
	}
	repeated, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", args)
	if err != nil || repeated.Artifact.RunID != run.ID {
		t.Fatal("tool replay duplicated candidate", err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", copilotArgs(map[string]string{"text": "different", "mode": "chapter"})); !IsConflict(err) {
		t.Fatal("reused call ID accepted different output", err)
	}
	diff, err := s.ExecuteCopilotTool(ctx, scope, "show_diff", copilotArgs(map[string]string{"runId": run.ID}))
	if err != nil {
		t.Fatal(err)
	}
	var before, after []string
	for _, h := range diff.Artifact.Diff {
		if h.Before != "" {
			before = append(before, h.Before)
		}
		if h.After != "" {
			after = append(after, h.After)
		}
	}
	if strings.Join(before, "\n\n") != r.PlainText || strings.Join(after, "\n\n") != run.Output.Text {
		t.Fatal("diff is not faithful")
	}
	adopted, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: r.ID, Document: DocumentFromText(run.Output.Text), Source: "ai", RunID: run.ID})
	if err != nil {
		t.Fatal("author adoption failed", err)
	}
	if adopted.ID == r.ID || adopted.ParentID != r.ID {
		t.Fatal("author adoption did not create revision")
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "show_diff", copilotArgs(map[string]string{"runId": run.ID})); !IsConflict(err) {
		t.Fatal("old diff allowed after author editing", err)
	}
}

func TestCopilotBlueprintProposalExplicitVersionedAdoption(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	scope := copilotScope(t, s, c, "")
	result, err := s.ExecuteCopilotTool(ctx, scope, "propose_blueprint", copilotArgs(map[string]string{"text": "场景目标：追查铜印；保留沈月的秘密；章末听到敲门声。"}))
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil || current.Blueprint != c.Blueprint {
		t.Fatal("proposal changed blueprint", err)
	}
	if result.Artifact.Kind != "blueprint-proposal" || result.Artifact.BlueprintVersion != c.BlueprintVersion {
		t.Fatal("blueprint source missing")
	}
	if _, err = s.ApplyCopilotBlueprint(ctx, result.Artifact.RunID, c.BlueprintVersion-1); !IsConflict(err) {
		t.Fatal("stale expected blueprint accepted", err)
	}
	saved, err := s.ApplyCopilotBlueprint(ctx, result.Artifact.RunID, c.BlueprintVersion)
	if err != nil {
		t.Fatal(err)
	}
	if saved.BlueprintVersion != c.BlueprintVersion+1 || saved.Blueprint != result.Artifact.Text || saved.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("blueprint adoption changed manuscript or not saved")
	}
	if _, err = s.ApplyCopilotBlueprint(ctx, result.Artifact.RunID, saved.BlueprintVersion); !IsConflict(err) {
		t.Fatal("old source candidate applied twice", err)
	}
}

func TestCopilotToolsScopeUnknownArgsAndStaleSourcesRejected(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	scope := copilotScope(t, s, c, "")
	for _, args := range []json.RawMessage{[]byte(`{"projectId":"foreign"}`), []byte(`null`), []byte(`{} {}`), []byte(`{"maxChars":-1}`)} {
		if _, err := s.ExecuteCopilotTool(ctx, scope, "read_chapter", args); err == nil {
			t.Fatalf("invalid args accepted %s", args)
		}
	}
	if _, err := s.ExecuteCopilotTool(ctx, scope, "bash", []byte(`{}`)); err == nil {
		t.Fatal("arbitrary execution accepted")
	}
	other, err := s.CreateProject(ctx, "外作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	wrong := scope
	wrong.ProjectID = other.ID
	if _, err = s.ExecuteCopilotTool(ctx, wrong, "read_chapter", []byte(`{}`)); err == nil {
		t.Fatal("foreign chapter scope accepted")
	}
	if _, err = s.SaveStudioBrief(ctx, c.ProjectID, StudioWrite[StoryBrief]{Data: StoryBrief{Genre: "悬疑", WorldRules: []string{}, Volumes: []StoryVolume{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_chapter", []byte(`{}`)); !IsConflict(err) {
		t.Fatal("changed Studio source accepted", err)
	}
}

func TestCopilotToolsReadOnlyRolesAndRevokedMembership(t *testing.T) {
	s, c := runtimeFixture(t)
	admin := copilotAdmin()
	u, uctx := collabUser(t, s, "copilot-reader")
	if _, err := s.AddMember(admin, c.ProjectID, u.Username, "viewer"); err != nil {
		t.Fatal(err)
	}
	scope, err := s.NewCopilotToolContext(uctx, job.NewID(), c.ProjectID, c.ID, c.CurrentRevisionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCopilotTool(context.Background(), scope, "read_chapter", []byte(`{}`)); err != nil {
		t.Fatal("viewer read failed", err)
	}
	for _, tool := range []string{"create_revision_candidate", "propose_blueprint"} {
		if _, err = s.ExecuteCopilotTool(context.Background(), scope, tool, []byte(`{"text":"新候选","mode":"chapter"}`)); !errors.Is(err, ErrForbidden) {
			t.Fatal("viewer wrote artifact", tool, err)
		}
	}
	if err = s.ChangeMember(admin, c.ProjectID, u.ID, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCopilotTool(context.Background(), scope, "read_chapter", []byte(`{}`)); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked member retained tool access", err)
	}
}

func TestCopilotMemoryToolsRespectCharacterKnowledgeAndFutureBoundary(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := copilotAdmin()
	if _, err := f.s.SaveSetting(ctx, f.p.ID, Setting{Kind: "world", Name: "秘密资料", Content: "绝密：铜印真正来自司钥署"}); err != nil {
		t.Fatal(err)
	}
	c1 := f.chapter(t, "第一章", "沈月知道铜印来自司钥署，林川只知道铜印与王都有关。")
	f.apply(t, f.proposal(t, c1, []MemoryChange{f.change(t, c1, "Knowledge", f.secret.ID, "origin", "铜印与王都有关", f.lin.ID, "Unknown", "林川只知道铜印与王都有关"), f.change(t, c1, "Knowledge", f.secret.ID, "origin", "铜印来自司钥署", f.shen.ID, "True", "沈月知道铜印来自司钥署")}))
	c2, err := f.s.CreateChapter(ctx, f.p.ID, "第二章", "")
	if err != nil {
		t.Fatal(err)
	}
	c3 := f.chapter(t, "第三章", "未来章节：司钥署的秘密将在第七天公开。")
	f.apply(t, f.proposal(t, c3, nil))
	scope, err := f.s.NewCopilotToolContext(ctx, job.NewID(), f.p.ID, c2.ID, c2.CurrentRevisionID, "", f.lin.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.s.ExecuteCopilotTool(ctx, scope, "search_story_memory", []byte(`{"query":"王都"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result.Content)
	if !strings.Contains(string(raw), "铜印与王都有关") || strings.Contains(string(raw), "司钥署") || strings.Contains(string(raw), "未来章节") {
		t.Fatalf("character memory leaked: %s", raw)
	}
	if _, err = f.s.ExecuteCopilotTool(ctx, scope, "search_story_memory", copilotArgs(map[string]string{"query": "司钥署", "characterId": f.shen.ID})); !errors.Is(err, ErrForbidden) {
		t.Fatal("POV override accepted", err)
	}
	settings, err := f.s.ExecuteCopilotTool(ctx, scope, "read_settings", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(settings.Content)
	if strings.Contains(string(raw), "真正来自") || strings.Contains(string(raw), "绝密") {
		t.Fatal("settings narrator truth leaked")
	}
	check, err := f.s.ExecuteCopilotTool(ctx, scope, "check_consistency", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(check.Content)
	if strings.Contains(string(raw), "司钥署") || !strings.Contains(string(raw), `"semanticReviewPerformed":false`) {
		t.Fatal("local check leaked or claimed semantic review", string(raw))
	}
	author := scope
	author.PointOfViewCharacterID = ""
	historical, err := f.s.ExecuteCopilotTool(ctx, author, "search_story_memory", []byte(`{"query":"铜印"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(historical.Content)
	if strings.Contains(string(raw), "未来章节") || strings.Contains(string(raw), "第七天") {
		t.Fatal("future text leaked", string(raw))
	}
}

func TestCopilotToolsBoundReadAndDiffWork(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	text := strings.Repeat("中文正文。", 1000)
	r, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(text)})
	if err != nil {
		t.Fatal(err)
	}
	c.CurrentRevisionID = r.ID
	scope := copilotScope(t, s, c, "")
	result, err := s.ExecuteCopilotTool(ctx, scope, "read_chapter", []byte(`{"offset":100,"maxChars":200}`))
	if err != nil {
		t.Fatal(err)
	}
	value := result.Content.(map[string]any)
	if value["truncated"] != true || value["nextOffset"] != 300 || len([]rune(value["text"].(string))) != 200 {
		t.Fatal("read was not bounded", value)
	}
	if len(s.CopilotTools()) != 9 {
		t.Fatal("tool catalog incomplete")
	}
	for _, tool := range s.CopilotTools() {
		if !json.Valid(tool.Parameters) {
			t.Fatal("invalid tool schema", tool.Name)
		}
	}
	hunks := copilotParagraphDiff(strings.Repeat("甲\n\n", 600), strings.Repeat("乙\n\n", 600))
	if len(hunks) != 1 || !hunks[0].Changed {
		t.Fatal("diff did not bound quadratic work")
	}
}

func TestCopilotCandidateCannotBeCreatedAfterTurnCancellation(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	session, err := s.CreateCopilotSession(ctx, c.ProjectID, CopilotSessionInput{ChapterID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "讨论改写", ChapterID: c.ID, InputRevisionID: c.CurrentRevisionID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE wb_copilot_turns SET status='Running',lease_owner='test-worker',lease_until=now()+interval '30 seconds' WHERE id=$1", turn.ID); err != nil {
		t.Fatal(err)
	}
	scope, err := s.NewCopilotToolContext(ctx, turn.ID, c.ProjectID, c.ID, c.CurrentRevisionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelCopilotTurn(ctx, turn.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"text":"取消后的候选不可保存。","mode":"chapter"}`)); !IsConflict(err) {
		t.Fatal("cancelled turn wrote candidate", err)
	}
	var count int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_runs WHERE project_id=$1", c.ProjectID).Scan(&count); err != nil || count != 0 {
		t.Fatal("artifact side effect after cancel", count, err)
	}
}
