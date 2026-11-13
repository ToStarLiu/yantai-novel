package workbench

import (
	"context"
	"strings"
	"testing"
)

func TestDeterministicReviewUsesVerbatimSignalsAndExplicitRules(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	_, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "rule", Name: "明确内容限制", Content: `{"forbiddenPhrases":["左臂瞬间痊愈"],"requiredPhrases":["蜡封未破"]}`})
	if err != nil {
		t.Fatal(err)
	}
	para := "林川沿着废磨坊的石墙缓缓走动，铜印始终留在胸前内袋。"
	text := para + "\n\n" + para + "\n左臂瞬间痊愈。\n" + strings.Repeat("雨", 550) + "。"
	r := saveCore(t, s, c, text)
	issues, err := s.DeterministicReview(ctx, c.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, issue := range issues {
		if issue.Quote == "" || !strings.Contains(text, issue.Quote) {
			t.Fatalf("invented quote %#v", issue)
		}
		kinds[issue.Kind]++
	}
	if kinds["Repetition"] == 0 || kinds["Readability"] != 1 || kinds["Planning"] != 1 || kinds["Rule"] != 2 {
		t.Fatalf("signals %#v", kinds)
	}
	_, other := setupCore(t, s)
	if _, err = s.DeterministicReview(ctx, c.ID, other.CurrentRevisionID); err != ErrNotFound {
		t.Fatalf("cross chapter accepted %v", err)
	}
	// Ordinary style words or a phrase mentioned once do not constitute a score.
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	r = saveCore(t, s, current, "他不由得抬起头。蜡封未破。")
	issues, err = s.DeterministicReview(ctx, c.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		if issue.Kind != "Planning" {
			t.Fatalf("unfounded style detection %#v", issue)
		}
	}
}

func TestStyleTemplatesAreIndependentAuthorEditablePolicies(t *testing.T) {
	profiles := StyleTemplates()
	if len(profiles) != 5 {
		t.Fatalf("profiles=%d", len(profiles))
	}
	seen := map[string]bool{}
	for _, p := range profiles {
		if p.ID == "" || seen[p.ID] || p.Name == "" || p.Description == "" || p.Instructions == "" {
			t.Fatalf("invalid profile %#v", p)
		}
		seen[p.ID] = true
	}
	profiles[0].Instructions = "edited"
	if StyleTemplates()[0].Instructions == "edited" {
		t.Fatal("caller mutated global template")
	}
}

func TestStoryMapOnlyShowsConfirmedValidFactsWithSources(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	person, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "character", Name: "林川"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.SaveSetting(ctx, p.ID, Setting{Kind: "world", Name: "铜印"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateProject(ctx, "另一本", "", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.SaveSetting(ctx, other.ID, Setting{Kind: "character", Name: "外来者"})
	if err != nil {
		t.Fatal(err)
	}
	r := saveCore(t, s, c, "林川把铜印留在内袋。林川误以为铜印已经烧毁。")
	if _, err = s.Finalize(ctx, c.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	mc, err := s.MemoryInput(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	changes := []MemoryChange{
		{Kind: "Ownership", Subject: item.ID, Predicate: "holder", Object: person.ID, TruthStatus: "True", Evidence: Evidence{RevisionID: r.ID, PlainTextHash: r.Hash, Quote: "林川把铜印留在内袋。"}},
		{Kind: "Knowledge", Subject: item.ID, Predicate: "destroyed", Object: "铜印已经烧毁", CharacterID: person.ID, TruthStatus: "False", Evidence: Evidence{RevisionID: r.ID, PlainTextHash: r.Hash, Quote: "林川误以为铜印已经烧毁。"}},
	}
	proposal, err := s.CreateProposal(ctx, c.ID, r.ID, mc.BaseStateID, mc.Epoch, changes)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.StoryMap(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Timeline) != 0 {
		t.Fatal("unconfirmed proposal became graph fact")
	}
	decisions := []MemoryDecision{}
	for _, change := range proposal.Changes {
		decisions = append(decisions, MemoryDecision{ID: change.ID, Decision: "Accept"})
	}
	if _, err = s.DecideProposal(ctx, proposal.ID, decisions); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyProposal(ctx, proposal.ID, mc.BaseStateID, r.ID); err != nil {
		t.Fatal(err)
	}
	graph, err := s.StoryMap(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Timeline) != 2 || len(graph.Relations) != 1 {
		t.Fatalf("graph %#v", graph)
	}
	for _, n := range graph.Nodes {
		if n.ID == foreign.ID {
			t.Fatal("foreign node leaked")
		}
	}
	relation := graph.Relations[0]
	if relation.Source != item.ID || relation.Target != person.ID || relation.SourceRevisionID != r.ID {
		t.Fatalf("relation %#v", relation)
	}
	for _, e := range graph.Timeline {
		if e.Evidence.RevisionID != r.ID || !strings.Contains(r.PlainText, e.Evidence.Quote) {
			t.Fatalf("source missing %#v", e)
		}
	}
	current, err := s.GetChapter(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := saveCore(t, s, current, "林川没有铜印。")
	if _, err = s.Finalize(ctx, c.ID, changed.ID); err != nil {
		t.Fatal(err)
	}
	graph, err = s.StoryMap(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Timeline) != 0 || len(graph.Relations) != 0 {
		t.Fatal("invalidated facts leaked into map")
	}
}
