package workbench

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectPackageRoundTripPreservesHistoryAndRemapsIDs(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	r := saveCore(t, s, c, "中文正文\n第二段。")
	if _, err := s.Finalize(ctx, c.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	setting, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "world", Name: "北岸磨坊", Content: "废弃"})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := s.ExportProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"LLM_API_KEY", "DATABASE_URL", "csrf_token", "Authorization"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("package includes %s", forbidden)
		}
	}
	restored, err := s.ImportProject(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Project.ID == p.ID || len(restored.Chapters) != 1 || len(restored.Settings) != 1 || restored.Settings[0].ID == setting.ID {
		t.Fatalf("restore ids %#v", restored)
	}
	got, err := s.GetChapter(ctx, restored.Chapters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision.PlainText != r.PlainText || got.CurrentRevisionID == r.ID || got.FinalRevisionID != got.CurrentRevisionID {
		t.Fatalf("restore current/final %#v", got)
	}
	revisions, err := s.ListRevisions(ctx, got.ID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("history %d %v", len(revisions), err)
	}
	markdown, err := s.ExportMarkdown(ctx, restored.Project.ID)
	if err != nil || !strings.Contains(markdown, "中文正文") {
		t.Fatalf("markdown %q %v", markdown, err)
	}
}

func TestProjectPackageRejectsForeignSourcesAndTamperingAtomically(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	_ = saveCore(t, s, c, "原文")
	original, err := s.ExportProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	cases := []func(*ProjectPackage){
		func(p *ProjectPackage) { p.FormatVersion = 999 },
		func(p *ProjectPackage) { p.Revisions[0].ChapterID = "foreign" },
		func(p *ProjectPackage) { p.Chapters[0].CurrentRevisionID = "foreign" },
		func(p *ProjectPackage) { p.Revisions[0].PlainText = "tampered" },
		func(p *ProjectPackage) { p.Revisions[0].ParentID = p.Revisions[0].ID },
		func(p *ProjectPackage) { p.Chapters[0].ID = p.Project.ID },
	}
	for i, mutate := range cases {
		raw, _ := json.Marshal(original)
		var p ProjectPackage
		_ = json.Unmarshal(raw, &p)
		mutate(&p)
		if _, err = s.ImportProject(ctx, p); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	all, err := s.ListProjects(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("partial import leaked project %d %v", len(all), err)
	}
}

func TestTextImportPreviewAndConfirm(t *testing.T) {
	input := "第1章 断桥\n林川左臂受伤。\n\n第2章 磨坊\n铜印仍在内袋。"
	preview, err := PreviewTextImport(input)
	if err != nil || len(preview) != 2 {
		t.Fatalf("preview %v %v", preview, err)
	}
	plain, err := PreviewTextImport("没有标题的中文正文")
	if err != nil || len(plain) != 1 || plain[0].Text != "没有标题的中文正文" {
		t.Fatal("unsplit import not preserved")
	}
	s := newCore(t)
	p, err := s.CreateProject(context.Background(), "导入", "", "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.GetProject(context.Background(), p.ID)
	if err != nil || len(before.Chapters) != 0 {
		t.Fatal("preview changed project")
	}
	chapters, err := s.ImportText(context.Background(), p.ID, preview)
	if err != nil || len(chapters) != 2 {
		t.Fatalf("import %v %v", chapters, err)
	}
	if chapters[0].Revision.Source != "import" || chapters[0].Revision.PlainText != "林川左臂受伤。" {
		t.Fatal("import revision mismatch")
	}
}
