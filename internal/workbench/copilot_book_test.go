package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"novel/internal/job"
	"novel/internal/testutil"
)

func bookCopilotFixture(t *testing.T) (*Service, Project, CopilotSession) {
	t.Helper()
	s := &Service{DB: testutil.Database(t), Model: "fake-v1", ProviderName: "fake", MaxOutputTokens: 500, MaxConcurrent: 1, JobTimeout: time.Second}
	p, err := s.CreateProject(copilotAdmin(), "尚未写正文的新书", "一个关于失忆与契约的故事", "幻想")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateCopilotSession(copilotAdmin(), p.ID, CopilotSessionInput{Title: "先规划全书", Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	return s, p, session
}
func bookTurn(t *testing.T, s *Service, session CopilotSession) CopilotTurn {
	t.Helper()
	turn, err := s.SubmitCopilotTurn(copilotAdmin(), session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "一起规划主线和各卷目标", Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	return runningCopilot(t, s, turn)
}

func TestBookCopilotZeroChapterPlanAuthorAdoptionAndOriginalBriefPreserved(t *testing.T) {
	s, p, session := bookCopilotFixture(t)
	ctx := copilotAdmin()
	brief, err := s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{ExpectedVersion: 0, Data: StoryBrief{Genre: "幻想", Audience: "成年读者", CoreConflict: "契约与自由", Restrictions: "不能复活死者", WorldRules: []string{"每次施法失去一段记忆"}, Volumes: []StoryVolume{}}})
	if err != nil {
		t.Fatal(err)
	}
	turn := bookTurn(t, s, session)
	read, err := s.ExecuteBookCopilotTool(ctx, turn, "read_book_brief", "read1", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(read.Content)
	if !strings.Contains(string(raw), "不能复活死者") {
		t.Fatal("author constraints omitted")
	}
	result, err := s.ExecuteBookCopilotTool(ctx, turn, "propose_book_plan", "call1", []byte(`{"text":"第一卷：订立契约。第二卷：发现代价。第三卷：以放弃力量换回自由。","title":"契约的代价"}`))
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := s.ExecuteBookCopilotTool(ctx, turn, "propose_book_plan", "call1", []byte(`{"text":"第一卷：订立契约。第二卷：发现代价。第三卷：以放弃力量换回自由。","title":"契约的代价"}`))
	if err != nil || repeated.Artifact.ProposalID != result.Artifact.ProposalID {
		t.Fatal("duplicate artifact", err)
	}
	if _, err = s.ExecuteBookCopilotTool(ctx, turn, "propose_book_plan", "call1", []byte(`{"text":"其他方案"}`)); !IsConflict(err) {
		t.Fatal("reused call changed content", err)
	}
	before, err := s.Studio(ctx, p.ID)
	if err != nil || before.Brief.Data.PlotPlan != "" {
		t.Fatal("AI changed brief automatically", err)
	}
	if _, err = s.ApplyCopilotBookPlan(ctx, result.Artifact.ProposalID, brief.Version); !IsConflict(err) {
		t.Fatal("running turn adopted", err)
	}
	if err = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	saved, err := s.ApplyCopilotBookPlan(ctx, result.Artifact.ProposalID, brief.Version)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Data.PlotPlan != result.Artifact.Text || saved.Data.Restrictions != "不能复活死者" || saved.Data.Audience != "成年读者" {
		t.Fatal("existing author setup lost", saved)
	}
	again, err := s.ApplyCopilotBookPlan(ctx, result.Artifact.ProposalID, brief.Version)
	if err != nil || again.Version != saved.Version {
		t.Fatal("adoption not idempotent", err)
	}
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil || len(detail.Chapters) != 0 {
		t.Fatal("book planning created a fake chapter", err)
	}
	for _, table := range []string{"wb_runs", "wb_revisions", "wb_facts"} {
		var count int
		if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal("planning wrote authoring data", table, count, err)
		}
	}
}

func TestBookCopilotCurrentSnapshotScopeAndViewerGuards(t *testing.T) {
	s, p, session := bookCopilotFixture(t)
	ctx := copilotAdmin()
	turn := bookTurn(t, s, session)
	if _, err := s.ExecuteBookCopilotTool(ctx, turn, "read_chapter", "call1", []byte(`{}`)); err == nil {
		t.Fatal("chapter tool exposed to book session")
	}
	result, err := s.ExecuteBookCopilotTool(ctx, turn, "propose_book_plan", "call1", []byte(`{"text":"可编辑的全书方案"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{ExpectedVersion: 0, Data: StoryBrief{Theme: "作者更改了方向"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyCopilotBookPlan(ctx, result.Artifact.ProposalID, 0); !IsConflict(err) {
		t.Fatal("stale book plan adopted", err)
	}
	if _, err = s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "范围切换", ChapterID: "foreign", Scope: "book"}); err == nil {
		t.Fatal("book accepted foreign chapter")
	}
	user, err := s.CreateUser(ctx, "book-viewer", "book-viewer-password", "书籍只读者")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddMember(ctx, p.ID, user.Username, "viewer"); err != nil {
		t.Fatal(err)
	}
	viewer := WithPrincipal(context.Background(), Principal{ID: user.ID, Username: user.Username})
	private, err := s.CreateCopilotSession(viewer, p.ID, CopilotSessionInput{Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	vturn, err := s.SubmitCopilotTurn(viewer, private.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "只读讨论"})
	if err != nil {
		t.Fatal(err)
	}
	vturn = runningCopilot(t, s, vturn)
	if _, err = s.ExecuteBookCopilotTool(viewer, vturn, "read_book_brief", "v1", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteBookCopilotTool(viewer, vturn, "propose_book_plan", "v2", []byte(`{"text":"越权方案"}`)); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer proposed plan", err)
	}
	if _, err = s.GetCopilotSession(ctx, private.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("admin read private planning discussion", err)
	}
}

func TestBookCopilotRealPiHarnessNoChapter(t *testing.T) {
	node, err := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := filepath.Abs("../../harness/dist/main.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err = os.Stat(entry); err != nil {
		t.Skip("Build Pi harness")
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	s, p, session := bookCopilotFixture(t)
	turn := bookTurn(t, s, session)
	ctx := copilotAdmin()
	if err = s.ExecuteCopilotTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	completed, err := s.GetCopilotTurn(ctx, turn.ID)
	if err != nil || completed.Status != "Succeeded" {
		t.Fatalf("book Pi turn failed %s %s %v", completed.Status, completed.Error, err)
	}
	if !strings.Contains(string(completed.Artifacts), "book-plan-proposal") {
		t.Fatalf("no book plan artifact %s", completed.Artifacts)
	}
	var artifacts []*CopilotArtifact
	if err = json.Unmarshal(completed.Artifacts, &artifacts); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		if artifact == nil {
			t.Fatal("read-only tool appended null artifact")
		}
	}
	view, err := s.GetCopilotSession(ctx, session.ID)
	if err != nil || !strings.Contains(string(view.Messages), "read_book_brief") {
		t.Fatal("real Pi book tools not persisted", err)
	}
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil || len(detail.Chapters) != 0 {
		t.Fatal("fake chapter created", err)
	}
}

func TestBookCopilotBoundedSourcesCanContinueAndDirectoryChangesInvalidate(t *testing.T) {
	s, p, session := bookCopilotFixture(t)
	ctx := copilotAdmin()
	plan := strings.Repeat("甲", 1800) + "最后的关键结局"
	if _, err := s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{ExpectedVersion: 0, Data: StoryBrief{PlotPlan: plan, Restrictions: strings.Repeat("规", 300) + "最重要的禁止事项"}}); err != nil {
		t.Fatal(err)
	}
	var target Setting
	for i := 0; i < 24; i++ {
		item, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "world", Name: fmt.Sprintf("地点%02d", i), Content: strings.Repeat("乙", 1000) + "隐藏港口"})
		if err != nil {
			t.Fatal(err)
		}
		target = item
	}
	turn := bookTurn(t, s, session)
	page, err := s.ExecuteBookCopilotTool(ctx, turn, "read_book_brief", "brief-page", []byte(`{"plotOffset":1800}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.Content)
	if !strings.Contains(string(raw), "最后的关键结局") {
		t.Fatal("long plot lost its tail", string(raw))
	}
	field, err := s.ExecuteBookCopilotTool(ctx, turn, "read_book_brief", "rules-page", []byte(`{"field":"restrictions","fieldOffset":300}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(field.Content.(map[string]any)["text"].(string), "最重要的禁止事项") {
		t.Fatal("long author constraint inaccessible", field)
	}
	settings, err := s.ExecuteBookCopilotTool(ctx, turn, "read_settings", "settings-page", copilotArgs(map[string]any{"id": target.ID, "contentOffset": 1000}))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(settings.Content)
	if !strings.Contains(string(raw), "隐藏港口") || strings.Contains(string(raw), strings.Repeat("乙", 20)) {
		t.Fatal("single setting continuation incorrect", string(raw))
	}
	if _, err = s.ExecuteBookCopilotTool(ctx, turn, "read_settings", "foreign", copilotArgs(map[string]any{"id": "another-project-setting"})); err != nil {
		t.Fatal(err)
	}
	proposal, err := s.ExecuteBookCopilotTool(ctx, turn, "propose_book_plan", "plan", []byte(`{"text":"根据当前目录拟定的方案"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, "UPDATE wb_projects SET description='作者直接更新了作品简介' WHERE id=$1", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyCopilotBookPlan(ctx, proposal.Artifact.ProposalID, 1); !IsConflict(err) {
		t.Fatal("changed metadata baseline adopted", err)
	}
}

func TestBookCopilotMemoryHasConfirmedFactsAndRoleBeliefProvenance(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := copilotAdmin()
	c := f.chapter(t, "第一章", "林川误以为铜印来自王都。叙述者尚未解释的秘密密码是天火。")
	f.apply(t, f.proposal(t, c, []MemoryChange{f.change(t, c, "Knowledge", f.seal.ID, "origin", "铜印来自王都", f.lin.ID, "False", "林川误以为铜印来自王都")}))
	pending := f.chapter(t, "第二章", "铜印能够唤醒神兽。这是尚未确认的抽取候选。")
	_ = f.proposal(t, pending, []MemoryChange{f.change(t, pending, "Foreshadow", f.seal.ID, "ability", "铜印能够唤醒神兽", "", "True", "铜印能够唤醒神兽")})
	f.s.ProviderName = "fake"
	f.s.Model = "fake-v1"
	f.s.MaxOutputTokens = 500
	session, err := f.s.CreateCopilotSession(ctx, f.p.ID, CopilotSessionInput{Scope: "book"})
	if err != nil {
		t.Fatal(err)
	}
	turn := bookTurn(t, f.s, session)
	result, err := f.s.ExecuteBookCopilotTool(ctx, turn, "search_story_memory", "memory", []byte(`{"query":"铜印"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result.Content)
	if !strings.Contains(string(raw), "铜印来自王都") || !strings.Contains(string(raw), `"truthStatus":"False"`) || !strings.Contains(string(raw), f.lin.ID) {
		t.Fatal("knowledge provenance omitted", string(raw))
	}
	if strings.Contains(string(raw), "天火") || strings.Contains(string(raw), "唤醒神兽") {
		t.Fatal("unconfirmed fact or unrelated body leaked", string(raw))
	}
}
