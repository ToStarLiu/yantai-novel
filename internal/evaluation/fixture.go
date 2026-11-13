// Package evaluation loads frozen, explicitly allowlisted model inputs.
// Answer keys and error annotations are intentionally outside this API.
package evaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const maxInputBytes = 8 << 20

type Chapter struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Outline            string   `json:"outline"`
	PreviousChapterIDs []string `json:"previousChapterIds"`
	Text               string   `json:"-"`
}

type Fixture struct {
	Version        string
	Story          string
	Chapters       []Chapter
	CorrectDraft   string
	IncorrectDraft string
}

type blueprint struct {
	SchemaVersion  string    `json:"schemaVersion"`
	FixtureVersion string    `json:"fixtureVersion"`
	DraftChapterID string    `json:"draftChapterId"`
	Chapters       []Chapter `json:"chapters"`
}

// Load reads only story.md, blueprints.json, the explicitly declared chapter
// files, and the two candidate drafts. It never enumerates the directory or
// reads expected-facts.json. All file access is confined to the fixture root.
func Load(root string) (*Fixture, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open fixture root: %w", err)
	}
	defer dir.Close()

	raw, err := readInput(dir, "blueprints.json")
	if err != nil {
		return nil, err
	}
	var bp blueprint
	if err := json.Unmarshal([]byte(raw), &bp); err != nil {
		return nil, fmt.Errorf("parse blueprints.json: %w", err)
	}
	if bp.SchemaVersion != "1" {
		return nil, fmt.Errorf("unsupported fixture schema version %q", bp.SchemaVersion)
	}
	if strings.TrimSpace(bp.FixtureVersion) == "" {
		return nil, errors.New("fixtureVersion is required")
	}
	indices, err := validateChapters(bp.Chapters)
	if err != nil {
		return nil, err
	}
	if bp.DraftChapterID != "03" {
		return nil, fmt.Errorf("stage 00 requires draftChapterId 03, got %q", bp.DraftChapterID)
	}
	if _, ok := indices[bp.DraftChapterID]; !ok {
		return nil, errors.New("draft chapter 03 is not declared")
	}

	fixture := &Fixture{Version: bp.FixtureVersion, Chapters: bp.Chapters}
	if fixture.Story, err = readInput(dir, "story.md"); err != nil {
		return nil, err
	}
	for i := range fixture.Chapters {
		chapter := &fixture.Chapters[i]
		if strings.TrimSpace(chapter.Title) == "" || strings.TrimSpace(chapter.Outline) == "" {
			return nil, fmt.Errorf("chapter %s requires title and outline", chapter.ID)
		}
		if chapter.Text, err = readInput(dir, "chapters/"+chapter.ID+".md"); err != nil {
			return nil, err
		}
	}
	if fixture.CorrectDraft, err = readInput(dir, "drafts/correct.md"); err != nil {
		return nil, err
	}
	if fixture.IncorrectDraft, err = readInput(dir, "drafts/incorrect.md"); err != nil {
		return nil, err
	}
	return fixture, nil
}

// Prefix returns only the target chapter's explicitly listed earlier chapters,
// in their declared narrative order. It excludes the target text, its outline,
// both candidate drafts, and any unlisted earlier or future chapters.
func (f *Fixture) Prefix(chapterID string) (string, error) {
	if f == nil {
		return "", errors.New("nil fixture")
	}
	indices, err := validateChapters(f.Chapters)
	if err != nil {
		return "", err
	}
	index, ok := indices[chapterID]
	if !ok {
		return "", fmt.Errorf("unknown chapter %q", chapterID)
	}
	texts := make([]string, 0, len(f.Chapters[index].PreviousChapterIDs))
	for _, id := range f.Chapters[index].PreviousChapterIDs {
		texts = append(texts, f.Chapters[indices[id]].Text)
	}
	return strings.Join(texts, "\n\n"), nil
}

func validateChapters(chapters []Chapter) (map[string]int, error) {
	if len(chapters) == 0 {
		return nil, errors.New("fixture must declare chapters")
	}
	indices := make(map[string]int, len(chapters))
	portableIDs := make(map[string]bool, len(chapters))
	for i, chapter := range chapters {
		if !safeChapterID(chapter.ID) {
			return nil, fmt.Errorf("unsafe chapter ID %q", chapter.ID)
		}
		key := strings.ToLower(chapter.ID)
		if portableIDs[key] {
			return nil, fmt.Errorf("duplicate chapter ID %q", chapter.ID)
		}
		portableIDs[key] = true
		indices[chapter.ID] = i
	}
	for i, chapter := range chapters {
		last := -1
		for _, id := range chapter.PreviousChapterIDs {
			previous, ok := indices[id]
			if !ok {
				return nil, fmt.Errorf("chapter %s references unknown previous chapter %q", chapter.ID, id)
			}
			if previous >= i {
				return nil, fmt.Errorf("chapter %s references non-previous chapter %s", chapter.ID, id)
			}
			if previous <= last {
				return nil, fmt.Errorf("chapter %s previous chapters must be unique and in narrative order", chapter.ID)
			}
			last = previous
		}
	}
	return indices, nil
}

func safeChapterID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		if i > 0 && (c == '-' || c == '_') {
			continue
		}
		return false
	}
	upper := strings.ToUpper(id)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9' {
		return false
	}
	return true
}

func readInput(root *os.Root, name string) (string, error) {
	file, err := root.Open(name)
	if err != nil {
		return "", fmt.Errorf("read fixture %s: %w", name, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat fixture %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("fixture %s must be a regular file", name)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return "", fmt.Errorf("read fixture %s: %w", name, err)
	}
	if len(raw) > maxInputBytes {
		return "", fmt.Errorf("fixture %s exceeds %d bytes", name, maxInputBytes)
	}
	if !utf8.Valid(raw) || strings.TrimSpace(string(raw)) == "" {
		return "", fmt.Errorf("fixture %s must contain nonempty UTF-8 text", name)
	}
	return string(raw), nil
}
