package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestProjectChapterTextPresenceTracksCurrentRevisionWithoutReturningManuscript(t *testing.T) {
	s := newCore(t)
	ctx := context.Background()
	project, chapter := setupCore(t, s)
	if chapter.CurrentRevisionID == "" || chapter.HasText == nil || *chapter.HasText {
		t.Fatalf("new chapter must have a revision but no text: %+v", chapter)
	}
	second, err := s.CreateChapter(ctx, project.ID, "第二章", "第一卷")
	if err != nil {
		t.Fatal(err)
	}
	assertSummary := func(wantText bool, currentID, finalID string) {
		t.Helper()
		detail, err := s.GetProject(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(detail.Chapters) != 2 || detail.Chapters[0].ID != chapter.ID || detail.Chapters[1].ID != second.ID || detail.Chapters[0].Order != 1 || detail.Chapters[1].Order != 2 {
			t.Fatalf("chapter ordering changed: %+v", detail.Chapters)
		}
		first := detail.Chapters[0]
		if first.HasText == nil || *first.HasText != wantText || first.CurrentRevisionID != currentID || first.FinalRevisionID != finalID {
			t.Fatalf("unexpected first chapter summary: %+v", first)
		}
		if detail.Chapters[1].HasText == nil || *detail.Chapters[1].HasText {
			t.Fatal("empty second chapter was counted as written")
		}
		for _, summary := range detail.Chapters {
			if summary.Revision != nil {
				t.Fatal("project summary included a manuscript revision")
			}
		}
		raw, err := json.Marshal(detail)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{`"revision":`, `"plainText":`, `"document":`, "正文不应出现在作品概览"} {
			if bytes.Contains(raw, []byte(private)) {
				t.Fatalf("project summary leaked manuscript field %s", private)
			}
		}
		var encoded struct {
			Chapters []struct {
				HasText *bool `json:"hasText"`
			} `json:"chapters"`
		}
		if err := json.Unmarshal(raw, &encoded); err != nil || len(encoded.Chapters) != 2 || encoded.Chapters[0].HasText == nil || *encoded.Chapters[0].HasText != wantText || encoded.Chapters[1].HasText == nil || *encoded.Chapters[1].HasText {
			t.Fatalf("hasText must be an explicit boolean, including false: %s (%v)", raw, err)
		}
		full, err := s.GetChapter(ctx, chapter.ID)
		if err != nil || full.HasText == nil || *full.HasText != wantText {
			t.Fatalf("single chapter and project summary disagree: %+v (%v)", full, err)
		}
	}
	assertSummary(false, chapter.CurrentRevisionID, "")
	text := saveCore(t, s, chapter, "\u3000正文不应出现在作品概览。\u00a0")
	if _, err := s.Finalize(ctx, chapter.ID, text.ID); err != nil {
		t.Fatal(err)
	}
	assertSummary(true, text.ID, text.ID)
	chapter.CurrentRevisionID = text.ID
	blank := saveCore(t, s, chapter, chapterTextWhitespace)
	assertSummary(false, blank.ID, text.ID)
}
