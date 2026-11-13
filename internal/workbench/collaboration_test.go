package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"novel/internal/job"
	"strings"
	"sync"
	"testing"
)

func collabUser(t *testing.T, s *Service, name string) (User, context.Context) {
	t.Helper()
	u, e := s.CreateUser(context.Background(), name, "test-password-strong", name)
	if e != nil {
		t.Fatal(e)
	}
	return u, WithPrincipal(context.Background(), Principal{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName})
}
func TestCollaborationUsersOwnersAndProjectIsolation(t *testing.T) {
	s := newCore(t)
	a, actx := collabUser(t, s, "alice")
	b, bctx := collabUser(t, s, "bob")
	p, e := s.CreateProject(actx, "Alice作品", "", "")
	if e != nil {
		t.Fatal(e)
	}
	other, e := s.CreateProject(bctx, "Bob作品", "", "")
	if e != nil {
		t.Fatal(e)
	}
	role, e := s.ProjectRole(actx, p.ID)
	if e != nil || role != "owner" {
		t.Fatal("creator not owner", e)
	}
	list, e := s.ListProjects(actx)
	if e != nil || len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("project leak %#v %v", list, e)
	}
	if e = s.RequireProjectRole(bctx, p.ID, "viewer"); !errors.Is(e, ErrForbidden) {
		t.Fatal("foreign access allowed")
	}
	if _, e = s.AddMember(bctx, p.ID, a.Username, "editor"); !errors.Is(e, ErrForbidden) {
		t.Fatal("foreign member management allowed")
	}
	if _, e = s.AddMember(actx, p.ID, b.Username, "editor"); e != nil {
		t.Fatal(e)
	}
	if e = s.RequireProjectRole(bctx, p.ID, "editor"); e != nil {
		t.Fatal(e)
	}
	if e = s.ChangeMember(actx, p.ID, a.ID, "viewer", false); !IsConflict(e) {
		t.Fatal("last owner demoted")
	}
	if e = s.ChangeMember(actx, p.ID, b.ID, "owner", false); e != nil {
		t.Fatal(e)
	}
	if e = s.ChangeMember(bctx, p.ID, a.ID, "", true); e != nil {
		t.Fatal(e)
	}
	if e = s.RequireProjectRole(actx, p.ID, "viewer"); !errors.Is(e, ErrForbidden) {
		t.Fatal("removed member still allowed")
	}
	if _, ok := s.AuthenticateUser(context.Background(), "bob", "test-password-strong"); !ok {
		t.Fatal("valid login failed")
	}
	if _, ok := s.AuthenticateUser(context.Background(), "bob", "wrong"); ok {
		t.Fatal("bad password accepted")
	}
	var hash string
	if e = s.DB.QueryRow(context.Background(), "SELECT password_hash FROM wb_users WHERE id=$1", b.ID).Scan(&hash); e != nil || !strings.HasPrefix(hash, "pbkdf2-sha256$") || strings.Contains(hash, "test-password") {
		t.Fatal("password not salted derived")
	}
	_ = other
}
func TestCollaborationInvitationSingleUseAndNoPrivilegeOverwrite(t *testing.T) {
	s := newCore(t)
	u, ctx := collabUser(t, s, "owner")
	p, e := s.CreateProject(ctx, "邀请作品", "", "")
	if e != nil {
		t.Fatal(e)
	}
	invite, e := s.CreateInvitation(ctx, p.ID, "reviewer")
	if e != nil {
		t.Fatal(e)
	}
	var stored string
	if e = s.DB.QueryRow(ctx, "SELECT token_hash FROM wb_invitations WHERE id=$1", invite.ID).Scan(&stored); e != nil || stored == invite.Token {
		t.Fatal("invitation token plaintext stored")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.AcceptInvitation(context.Background(), invite.Token, "reader", "strong-password-12", "读者")
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("single use successes %d", success)
	}
	existing, e := s.CreateInvitation(ctx, p.ID, "viewer")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AcceptInvitation(context.Background(), existing.Token, u.Username, "wrong", "x"); e == nil {
		t.Fatal("existing account stolen")
	}
	if _, e = s.AcceptInvitation(context.Background(), existing.Token, u.Username, "test-password-strong", "x"); e != nil {
		t.Fatal(e)
	}
	role, _ := s.ProjectRole(ctx, p.ID)
	if role != "owner" {
		t.Fatal("invitation downgraded owner")
	}
	revoked, e := s.CreateInvitation(ctx, p.ID, "viewer")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RevokeInvitation(ctx, p.ID, revoked.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AcceptInvitation(context.Background(), revoked.Token, "revoked", "strong-password-12", "revoked"); e == nil {
		t.Fatal("revoked invite accepted")
	}
}
func TestCollaborationPresenceTransactionalEventsAndImportOwnership(t *testing.T) {
	s := newCore(t)
	_, ctx := collabUser(t, s, "collaborator")
	p, e := s.CreateProject(ctx, "协作", "", "")
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.CreateChapter(ctx, p.ID, "一", "")
	if e != nil {
		t.Fatal(e)
	}
	presence, e := s.UpdatePresence(ctx, p.ID, PresenceInput{ChapterID: c.ID, RevisionID: c.CurrentRevisionID})
	if e != nil || len(presence) != 1 {
		t.Fatal(e, presence)
	}
	_, other := setupCore(t, s)
	if _, e = s.UpdatePresence(ctx, p.ID, PresenceInput{ChapterID: other.ID}); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign presence accepted")
	}
	before, e := s.CollaborationEvents(ctx, p.ID, 0)
	if e != nil || len(before) < 3 {
		t.Fatal("real events missing", e, before)
	}
	last := before[len(before)-1].ID
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, "UPDATE wb_projects SET title='rolled-back' WHERE id=$1", p.ID)
	if e != nil {
		t.Fatal(e)
	}
	_ = tx.Rollback(ctx)
	after, e := s.CollaborationEvents(ctx, p.ID, last)
	if e != nil || len(after) != 0 {
		t.Fatal("rollback emitted change", e, after)
	}
	pkg, e := s.ExportProject(ctx, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	imported, e := s.ImportProject(ctx, pkg)
	if e != nil {
		t.Fatal(e)
	}
	if role, e := s.ProjectRole(ctx, imported.Project.ID); e != nil || role != "owner" {
		t.Fatal("import owner missing", e)
	}
}
func TestAICandidateStudioVersionMustStillMatch(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	snapshot, e := s.StudioPromptContext(ctx, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	project, e := s.GetProject(ctx, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	runID := job.NewID()
	input, _ := json.Marshal(map[string]any{"studioVersions": snapshot.Versions, "blueprintVersion": c.BlueprintVersion, "stateVersionId": project.Project.StateVersionID})
	_, e = s.DB.Exec(ctx, "INSERT INTO wb_runs(id,project_id,chapter_id,type,status,input,intent_id,input_hash,epoch,revision_id) VALUES($1,$2,$3,'draft','Succeeded',$4,$5,'hash',$6,$7)", runID, p.ID, c.ID, input, job.NewID(), project.Project.Epoch, c.CurrentRevisionID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{Data: StoryBrief{CoreConflict: "变更主冲突"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("过期候选。"), Source: "ai", RunID: runID}); !IsConflict(e) {
		t.Fatal("stale Studio candidate accepted", e)
	}
}
