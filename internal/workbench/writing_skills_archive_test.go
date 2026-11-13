package workbench

import (
	"context"
	"strings"
	"testing"
)

func TestWritingSkillsBookArchiveRoundTripPreservesGuidesAndResources(t *testing.T) {
	s := newCore(t)
	ctx := context.Background()
	p, _ := setupCore(t, s)
	original, err := s.SaveWritingSkill(ctx, p.ID, WritingSkillSave{Preview: SkillPreview{Content: "---\nname: test-book-guide\ndescription: preserve book voice\n---\nUse exact observations and protect the unknown.\n", Resources: []WritingSkillResource{{Path: "references/voice.md", Content: "Prefer a concrete object over a labelled emotion."}}, Source: WritingSkillSource{URL: "https://example.com/guide", License: "MIT"}}, Enabled: true, Priority: 30, TaskTypes: []string{"draft", "copilot"}})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := s.ExportProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.WritingSkills) != 1 {
		t.Fatal("guides lost in export")
	}
	restored, err := s.ImportProject(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	guides, err := s.ListWritingSkills(ctx, restored.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(guides.Items) != 1 || guides.Version != 1 {
		t.Fatal("guides lost in import", guides)
	}
	got := guides.Items[0]
	if got.ID == original.ID || got.ProjectID != restored.Project.ID || got.Version != 1 || !got.Enabled || got.Priority != 30 || got.Content != original.Content || len(got.Resources) != 1 || got.Resources[0].Content != original.Resources[0].Content || got.Source.URL != original.Source.URL {
		t.Fatal("guide identity or configuration drifted", got)
	}
	snapshot, err := s.ResolveWritingSkills(ctx, restored.Project.ID, "draft")
	if err != nil || len(snapshot.Items) != 1 || snapshot.Items[0].Hash != original.Hash {
		t.Fatal("restored guide cannot be resolved", err)
	}
	archive.WritingSkills[0].Resources[0].Path = "../escape.md"
	before, _ := s.ListProjects(ctx)
	if _, err = s.ImportProject(ctx, archive); err == nil || !strings.Contains(err.Error(), "Skill") {
		t.Fatal("tampered guide path accepted", err)
	}
	after, _ := s.ListProjects(ctx)
	if len(after) != len(before) {
		t.Fatal("failed guide import left partial book")
	}
}
