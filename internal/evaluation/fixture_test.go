package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testBlueprint struct {
	SchemaVersion  string        `json:"schemaVersion"`
	FixtureVersion string        `json:"fixtureVersion"`
	DraftChapterID string        `json:"draftChapterId"`
	Chapters       []testChapter `json:"chapters"`
}

type testChapter struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Outline            string   `json:"outline"`
	PreviousChapterIDs []string `json:"previousChapterIds"`
}

func testFixture(t *testing.T) (string, testBlueprint) {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"chapters", "drafts", "expected-facts.json"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"story.md":                       "initial-story-only",
		"chapters/01.md":                 "first-chapter-evidence",
		"chapters/02.md":                 "second-chapter-evidence",
		"chapters/03.md":                 "current-chapter-must-not-be-prefix",
		"chapters/04.md":                 "future-unlisted-chapter",
		"drafts/correct.md":              "correct-third-candidate",
		"drafts/incorrect.md":            "incorrect-third-candidate",
		"expected-facts.json/answers.md": "evaluation-answer-secret",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bp := testBlueprint{
		SchemaVersion: "1", FixtureVersion: "test-v1", DraftChapterID: "03",
		Chapters: []testChapter{
			{ID: "01", Title: "one", Outline: "outline-one", PreviousChapterIDs: []string{}},
			{ID: "02", Title: "two", Outline: "outline-two", PreviousChapterIDs: []string{"01"}},
			{ID: "03", Title: "three", Outline: "outline-three", PreviousChapterIDs: []string{"01", "02"}},
		},
	}
	writeBlueprint(t, root, bp)
	return root, bp
}

func writeBlueprint(t *testing.T, root string, bp testBlueprint) {
	t.Helper()
	raw, err := json.Marshal(bp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blueprints.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadOnlyAllowlistedInputs(t *testing.T) {
	root, _ := testFixture(t)
	// expected-facts.json is deliberately a directory. Reading it would fail;
	// neither loading the fixture nor assembling model input needs it.
	fixture, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "test-v1" || fixture.Story != "initial-story-only" {
		t.Fatalf("wrong fixture metadata: %#v", fixture)
	}
	if len(fixture.Chapters) != 3 || fixture.Chapters[2].Text != "current-chapter-must-not-be-prefix" {
		t.Fatalf("wrong chapter data: %#v", fixture.Chapters)
	}
	if fixture.CorrectDraft != "correct-third-candidate" || fixture.IncorrectDraft != "incorrect-third-candidate" {
		t.Fatal("candidate drafts not preserved separately")
	}
	prefix, err := fixture.Prefix("03")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"first-chapter-evidence", "second-chapter-evidence"} {
		if !strings.Contains(prefix, required) {
			t.Fatalf("missing allowed source %q in prefix %q", required, prefix)
		}
	}
	for _, forbidden := range []string{"current-chapter", "future-unlisted", "evaluation-answer-secret", "initial-story", "outline-three", "third-candidate"} {
		if strings.Contains(prefix, forbidden) {
			t.Fatalf("disallowed input %q leaked into prefix %q", forbidden, prefix)
		}
	}
	if strings.Index(prefix, "first-chapter") >= strings.Index(prefix, "second-chapter") {
		t.Fatal("prefix must follow narrative order")
	}
	firstPrefix, err := fixture.Prefix("01")
	if err != nil || firstPrefix != "" {
		t.Fatalf("first chapter has no prefix, got %q, %v", firstPrefix, err)
	}
	if _, err := fixture.Prefix("missing"); err == nil {
		t.Fatal("unknown target chapter must fail")
	}
}

func TestLoadRejectsUnsafeChapterIDs(t *testing.T) {
	ids := []string{"", "..", "../outside", `..\outside`, "/outside", `C:\outside`, "C:outside", "01:stream", "a/b", `a\b`, "a.b", "CON", "nul", "COM1", "LPT9"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			root, bp := testFixture(t)
			bp.Chapters[0].ID = id
			writeBlueprint(t, root, bp)
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "unsafe chapter ID") {
				t.Fatalf("unsafe chapter ID %q must fail validation before file access: %v", id, err)
			}
		})
	}
}

func TestLoadRejectsInvalidDependencies(t *testing.T) {
	cases := []struct {
		name    string
		ids     []string
		message string
	}{
		{"unknown", []string{"99"}, "unknown previous chapter"},
		{"future", []string{"03"}, "non-previous chapter"},
		{"self", []string{"02"}, "non-previous chapter"},
		{"duplicate", []string{"01", "01"}, "unique and in narrative order"},
		{"traversal", []string{"../outside"}, "unknown previous chapter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, bp := testFixture(t)
			bp.Chapters[1].PreviousChapterIDs = tc.ids
			writeBlueprint(t, root, bp)
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("invalid dependencies %v must fail validation: %v", tc.ids, err)
			}
		})
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	root, bp := testFixture(t)
	bp.Chapters[1].ID = "01"
	writeBlueprint(t, root, bp)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "duplicate chapter ID") {
		t.Fatalf("duplicate chapter IDs must fail validation: %v", err)
	}
}

func TestLoadRequiresDeclaredThirdDraftChapter(t *testing.T) {
	for _, draftID := range []string{"", "02", "99", "../03"} {
		t.Run(draftID, func(t *testing.T) {
			root, bp := testFixture(t)
			bp.DraftChapterID = draftID
			writeBlueprint(t, root, bp)
			if _, err := Load(root); err == nil {
				t.Fatalf("invalid draftChapterId %q accepted", draftID)
			}
		})
	}
	t.Run("missing third chapter", func(t *testing.T) {
		root, bp := testFixture(t)
		bp.Chapters = bp.Chapters[:2]
		writeBlueprint(t, root, bp)
		if _, err := Load(root); err == nil {
			t.Fatal("missing third draft chapter accepted")
		}
	})
}

func TestPrefixUsesExplicitDependencies(t *testing.T) {
	root, bp := testFixture(t)
	bp.Chapters[2].PreviousChapterIDs = []string{"02"}
	writeBlueprint(t, root, bp)
	fixture, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := fixture.Prefix("03")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prefix, "second-chapter-evidence") || strings.Contains(prefix, "first-chapter-evidence") {
		t.Fatalf("prefix must only use explicit dependencies, got %q", prefix)
	}
}
