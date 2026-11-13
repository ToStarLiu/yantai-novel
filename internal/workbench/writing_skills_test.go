package workbench

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

const testWritingSkill = "---\nname: restrained-prose\ndescription: |\n  用动作和物象保留人物潜台词，避免解释已能看见的情绪。\nlicense: MIT\nallowed-tools: [Bash]\n---\n先阅读作者选段，再选取一个具体动作表现犹豫。保留人物知识边界。\n参考 references/dialogue.md。"

func TestWritingSkillParsingBundleAndUntrustedExecutionLimits(t *testing.T) {
	preview, err := parseWritingSkill(testWritingSkill, []WritingSkillResource{{"references/dialogue.md", "不要用旁白重复解释人物对白。"}, {"scripts/runner.py", "print('never run')"}}, WritingSkillSource{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Name != "restrained-prose" || !strings.Contains(preview.Description, "潜台词") || len(preview.Resources) != 1 || preview.Hash == "" {
		t.Fatal("parsed content missing", preview)
	}
	if !strings.Contains(strings.Join(preview.Warnings, " "), "不会授权工具") {
		t.Fatal("tool declaration not marked ignored")
	}
	if strings.Contains(preview.Content, "never run") {
		t.Fatal("script was evaluated or mixed into instructions")
	}
	for _, bad := range []string{"no frontmatter", "---\nname: ../bad\ndescription: valid\n---\nbody", "---\nname: valid\ndescription: valid\nname: dup\n---\nbody", "---\nname: valid\ndescription: hi\n---\n"} {
		if _, err = parseWritingSkill(bad, nil, WritingSkillSource{}); err == nil {
			t.Fatal("invalid Skill accepted", bad)
		}
	}
	for _, bad := range []string{"../evil.md", "/tmp/evil.md", "C:/evil.md", "references/../secret.md", "references\\secret.md"} {
		if _, err = parseWritingSkill(testWritingSkill, []WritingSkillResource{{bad, "bad"}}, WritingSkillSource{}); err == nil {
			t.Fatal("unsafe resource accepted", bad)
		}
	}
	if _, err = parseWritingSkill(testWritingSkill, []WritingSkillResource{{"references/big.md", strings.Repeat("x", maxSkillDocument+1)}}, WritingSkillSource{}); err == nil {
		t.Fatal("large source accepted")
	}
	if _, err = parseWritingSkill(strings.Replace(testWritingSkill, "restrained-prose", strings.Repeat("a", 101), 1), nil, WritingSkillSource{}); err == nil {
		t.Fatal("long name accepted")
	}
}
func makeSkillZip(t *testing.T, files map[string]string) string {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(data.Bytes())
}
func TestWritingSkillZIPImportsOnlyRelativeDocumentResources(t *testing.T) {
	parsed, err := previewWritingSkillZip(makeSkillZip(t, map[string]string{"pack/SKILL.md": testWritingSkill, "pack/references/dialogue.md": "保留潜台词。", "pack/scripts/draft.py": "raise Exception('must not execute')", "other.md": "outside"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Resources) != 1 || parsed.Resources[0].Path != "references/dialogue.md" || len(parsed.Warnings) < 3 {
		t.Fatal("resource package semantics wrong", parsed)
	}
	for _, files := range []map[string]string{{"../SKILL.md": testWritingSkill}, {"SKILL.md": testWritingSkill, "../../escape.md": "bad"}, {"a/SKILL.md": testWritingSkill, "b/SKILL.md": testWritingSkill}, {"SKILL.md": testWritingSkill, "refs/big.md": strings.Repeat("x", maxSkillDocument+1)}} {
		if _, err = previewWritingSkillZip(makeSkillZip(t, files)); err == nil {
			t.Fatal("unsafe zip accepted")
		}
	}
	var data bytes.Buffer
	z := zip.NewWriter(&data)
	h := &zip.FileHeader{Name: "SKILL.md"}
	h.SetMode(os.ModeSymlink | 0777)
	f, _ := z.CreateHeader(h)
	f.Write([]byte(testWritingSkill))
	z.Close()
	if _, err = previewWritingSkillZip(base64.StdEncoding.EncodeToString(data.Bytes())); err == nil {
		t.Fatal("symbolic link accepted")
	}
}
func TestWritingSkillGitHubLinksArePinnedSafeAndDirectoryScoped(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, u := range []string{"https://github.com/owner/repo/tree/" + sha + "/skills/scene", "https://github.com/owner/repo/blob/" + sha + "/skills/scene/SKILL.md", "https://raw.githubusercontent.com/owner/repo/" + sha + "/skills/scene/SKILL.md"} {
		o, r, ref, folder, err := githubSkillLocation(u)
		if err != nil || o != "owner" || r != "repo" || ref != sha || folder != "skills/scene" {
			t.Fatal("location parsing wrong", u, err)
		}
	}
	for _, u := range []string{"http://github.com/o/r/tree/main/skill", "https://evil.example/o/r/tree/main/skill", "https://github.com.evil.example/o/r/tree/main/skill", "https://user:secret@github.com/o/r/tree/main/skill", "https://github.com/o/r/tree/main/../../etc", "https://github.com:8443/o/r/tree/main/skill", "https://github.com/o/r/tree/main/skill?token=secret", "https://github.com/o/r/tree/main/skill#bad"} {
		if _, _, _, _, err := githubSkillLocation(u); err == nil {
			t.Fatal("unsafe import location accepted", u)
		}
	}
}
func TestWritingSkillsPersistVersionTaskRoutingResourcesAndAuthorControl(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	preview, err := s.PreviewWritingSkill(ctx, c.ProjectID, WritingSkillPreviewInput{Format: "markdown", Content: testWritingSkill})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListWritingSkills(ctx, c.ProjectID)
	if err != nil || len(list.Items) != 0 || list.Version != 0 {
		t.Fatal("preview mutated library", err)
	}
	preview.Resources = []WritingSkillResource{{"references/dialogue.md", strings.Repeat("潜台词", 1700)}}
	preview.Hash = "attacker supplied hash"
	preview.Name = "fake-name"
	saved, err := s.SaveWritingSkill(ctx, c.ProjectID, WritingSkillSave{Preview: preview, Enabled: true, Priority: 10, TaskTypes: []string{"copilot", "planning"}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "restrained-prose" || saved.Hash == "attacker supplied hash" || saved.Version != 1 {
		t.Fatal("trusted parsed metadata was not regenerated")
	}
	snap, err := s.ResolveWritingSkills(ctx, c.ProjectID, "copilot")
	if err != nil || snap.Version != 1 || snap.Hash == "" || len(snap.Items) != 1 {
		t.Fatal("active snapshot wrong", err)
	}
	draft, err := s.ResolveWritingSkills(ctx, c.ProjectID, "draft")
	if err != nil || len(draft.Items) != 0 || draft.Version != 1 {
		t.Fatal("task filtering wrong", err)
	}
	var reconstructed strings.Builder
	for offset := 0; offset < 5100; {
		page, err := s.ReadWritingSkillResource(ctx, c.ProjectID, saved.ID, 1, "references/dialogue.md", offset, 1000)
		if err != nil {
			t.Fatal(err)
		}
		reconstructed.WriteString(page.Text)
		if page.NextOffset != offset+len([]rune(page.Text)) {
			t.Fatal("resource cursor skipped chars")
		}
		offset = page.NextOffset
	}
	if reconstructed.String() != saved.Resources[0].Content {
		t.Fatal("resource paging lost contents")
	}
	modified := strings.Replace(saved.Content, "具体动作", "可观察的动作", 1)
	updated, err := s.UpdateWritingSkill(ctx, c.ProjectID, saved.ID, WritingSkillUpdate{ExpectedVersion: 1, Content: &modified})
	if err != nil || updated.Version != 2 || updated.Hash == saved.Hash {
		t.Fatal("immutable edit failed", err)
	}
	var old WritingSkill
	var raw []byte
	if err = s.DB.QueryRow(ctx, "SELECT snapshot FROM wb_writing_skill_versions WHERE skill_id=$1 AND version=1", saved.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(raw, &old)
	if old.Content != saved.Content {
		t.Fatal("old skill revision changed")
	}
	if _, err = s.UpdateWritingSkill(ctx, c.ProjectID, saved.ID, WritingSkillUpdate{ExpectedVersion: 1}); err == nil {
		t.Fatal("stale edit accepted")
	}
	if _, err = s.ReadWritingSkillResource(ctx, c.ProjectID, saved.ID, 1, "SKILL.md", 0, 1000); err == nil {
		t.Fatal("stale resource accepted")
	}
	disabled := false
	_, err = s.UpdateWritingSkill(ctx, c.ProjectID, saved.ID, WritingSkillUpdate{ExpectedVersion: 2, Enabled: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.ResolveWritingSkills(ctx, c.ProjectID, "copilot")
	if err != nil || empty.Version != 3 || empty.Hash != "" || len(empty.Items) != 0 {
		t.Fatal("disable erased change version", err)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	valid, err := ValidateWritingSkillsTx(ctx, tx, c.ProjectID, snap.Version)
	if err != nil || valid {
		t.Fatal("old skillset not invalidated", err)
	}
	after, err := s.GetChapter(ctx, c.ID)
	if err != nil || before.CurrentRevisionID != after.CurrentRevisionID {
		t.Fatal("skill import changed manuscript")
	}
	exported, _ := json.Marshal(updated.SkillPreview)
	again, err := s.PreviewWritingSkill(ctx, c.ProjectID, WritingSkillPreviewInput{Format: "bundle", Content: string(exported)})
	if err != nil || again.Hash != updated.Hash || len(again.Resources) != 1 {
		t.Fatal("bundle lost document provenance", err)
	}
}
func TestWritingSkillsProjectRolesIsolationLimitsAndDeletionHistory(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	preview, err := s.PreviewWritingSkill(ctx, c.ProjectID, WritingSkillPreviewInput{Content: testWritingSkill})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "别的作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveWritingSkill(ctx, c.ProjectID, WritingSkillSave{Preview: preview, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetWritingSkill(ctx, p.ID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross project skill read", err)
	}
	user, err := s.CreateUser(ctx, "skill-viewer", "long-test-password", "Viewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, "INSERT INTO wb_project_members(project_id,user_id,role) VALUES($1,$2,'viewer')", c.ProjectID, user.ID); err != nil {
		t.Fatal(err)
	}
	viewer := WithPrincipal(context.Background(), Principal{ID: user.ID})
	if _, err = s.ListWritingSkills(viewer, c.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewWritingSkill(viewer, c.ProjectID, WritingSkillPreviewInput{Content: testWritingSkill}); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer imported skill", err)
	}
	if err = s.DeleteWritingSkill(viewer, c.ProjectID, saved.ID, 1); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer deleted skill", err)
	}
	if _, err = s.ListWritingSkills(viewer, p.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign project read", err)
	}
	for i := 1; i < 6; i++ {
		p := preview
		p.Content = strings.Replace(testWritingSkill, "restrained-prose", fmt.Sprintf("skill-%d", i), 1)
		if _, err = s.SaveWritingSkill(ctx, c.ProjectID, WritingSkillSave{Preview: p, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	seventh := preview
	seventh.Content = strings.Replace(testWritingSkill, "restrained-prose", "seventh-skill", 1)
	if _, err = s.SaveWritingSkill(ctx, c.ProjectID, WritingSkillSave{Preview: seventh, Enabled: true}); err == nil {
		t.Fatal("enabled skill overflow accepted")
	}
	pending, err := s.SaveWritingSkill(ctx, c.ProjectID, WritingSkillSave{Preview: seventh, Enabled: false})
	if err != nil {
		t.Fatal("inactive library import blocked", err)
	}
	active := true
	if _, err = s.UpdateWritingSkill(ctx, c.ProjectID, pending.ID, WritingSkillUpdate{ExpectedVersion: 1, Enabled: &active}); err == nil {
		t.Fatal("overflow activation accepted")
	}
	if err = s.DeleteWritingSkill(ctx, c.ProjectID, saved.ID, 2); err == nil {
		t.Fatal("stale delete accepted")
	}
	if err = s.DeleteWritingSkill(ctx, c.ProjectID, saved.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetWritingSkill(ctx, c.ProjectID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted skill active", err)
	}
	var count int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_writing_skill_versions WHERE skill_id=$1", saved.ID).Scan(&count); err != nil || count != 2 {
		t.Fatal("delete destroyed skill history", err)
	}
	if _, err = s.UpdateWritingSkill(ctx, c.ProjectID, pending.ID, WritingSkillUpdate{ExpectedVersion: 1, Enabled: &active}); err != nil {
		t.Fatal("activation after freeing slot failed", err)
	}
	if _, err = s.SaveWritingSkill(ctx, c.ProjectID, WritingSkillSave{Preview: preview, Priority: 101}); err == nil {
		t.Fatal("invalid priority accepted")
	}
}

func TestWritingSkillFullPackageUpdateAndProvenanceCannotBeForged(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	source := WritingSkillSource{URL: "https://github.com/test/repo/tree/" + strings.Repeat("a", 40) + "/skill", Revision: strings.Repeat("a", 40), License: "MIT", Verification: "github-pinned"}
	forged, err := parseWritingSkill(testWritingSkill, nil, source)
	if err != nil {
		t.Fatal(err)
	}
	forged.Source.Verification = "github-pinned"
	forged.PreviewToken = strings.Repeat("0", 64)
	saved, err := s.SaveWritingSkill(ctx, c.ProjectID, WritingSkillSave{Preview: forged, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Source.Verification != "declared" {
		t.Fatal("client forged remote verification")
	}
	trusted, err := parseWritingSkill(testWritingSkill, []WritingSkillResource{{"references/new.md", "完整换版的资料"}}, source)
	if err != nil {
		t.Fatal(err)
	}
	trusted.Source.Verification = "github-pinned"
	trusted.PreviewToken = signWritingSkillPreview(trusted)
	updated, err := s.UpdateWritingSkill(ctx, c.ProjectID, saved.ID, WritingSkillUpdate{ExpectedVersion: 1, Preview: &trusted})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != saved.ID || updated.Version != 2 || len(updated.Resources) != 1 || updated.Resources[0].Content != "完整换版的资料" || updated.Source.Verification != "github-pinned" {
		t.Fatal("full package replacement failed", updated)
	}
	trusted.Resources[0].Content = "tampered after preview"
	updated, err = s.UpdateWritingSkill(ctx, c.ProjectID, saved.ID, WritingSkillUpdate{ExpectedVersion: 2, Preview: &trusted})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source.Verification != "declared" {
		t.Fatal("changed package reused remote attestation")
	}
	maliciousSource := source
	maliciousSource.URL = "javascript:alert(1)"
	if _, err = parseWritingSkill(testWritingSkill, nil, maliciousSource); err == nil {
		t.Fatal("executable source link accepted")
	}
	raw, _ := json.Marshal(trusted)
	preview, err := s.PreviewWritingSkill(ctx, c.ProjectID, WritingSkillPreviewInput{Format: "bundle", Content: string(raw)})
	if err != nil || preview.Source.Verification != "declared" {
		t.Fatal("imported JSON falsely claimed live verification", err)
	}
}
