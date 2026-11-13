package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrozenQuestionsMatchIndependentSourcesAndSplit(t *testing.T) {
	root := "../../evaluation/fixtures/retrieval-60"
	var story struct{ Chapters []Chapter }
	var qs struct{ Questions []Question }
	var relations struct{ Qrels []Relation }
	var split struct{ Development, Holdout []string }
	files := map[string]any{"story.json": &story, "questions.json": &qs, "qrels.json": &relations, "split.json": &split}
	for name, target := range files {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	if len(story.Chapters) != 20 || len(qs.Questions) != 60 || len(relations.Qrels) != 60 || len(split.Development) != 40 || len(split.Holdout) != 20 {
		t.Fatal("frozen fixture scale changed")
	}
	chapters := map[string]Chapter{}
	for _, c := range story.Chapters {
		chapters[c.ID] = c
	}
	labels := map[string]Relation{}
	for _, r := range relations.Qrels {
		labels[r.QuestionID] = r
	}
	splitByID := map[string]string{}
	for _, id := range split.Development {
		splitByID[id] = "development"
	}
	for _, id := range split.Holdout {
		if splitByID[id] != "" {
			t.Fatal("development/holdout overlap")
		}
		splitByID[id] = "holdout"
	}
	unknown := 0
	for _, q := range qs.Questions {
		if splitByID[q.ID] != q.Split {
			t.Fatalf("%s split mismatch", q.ID)
		}
		label, ok := labels[q.ID]
		if !ok {
			t.Fatal("missing label")
		}
		if !q.Answerable {
			unknown++
			if len(label.Evidence) > 0 {
				t.Fatal("unknown question has an answer label")
			}
			continue
		}
		if len(label.Evidence) == 0 {
			t.Fatal("answerable question has no source")
		}
		for _, e := range label.Evidence {
			if e.Quote == "" || !strings.Contains(chapters[e.ChapterID].Text, e.Quote) {
				t.Fatalf("%s invented evidence", q.ID)
			}
		}
	}
	if unknown < 10 {
		t.Fatal("too few no-answer or invisible questions")
	}
}

func TestAggregateSeparatesUnknownsFromRecallAndDoesNotHideErrors(t *testing.T) {
	one, zero := 1.0, 0.0
	results := []Result{{Split: "holdout", Answerable: true, Recall: &one}, {Split: "holdout", Answerable: true, Recall: &zero}, {Split: "holdout", Answerable: false, WrongEvidence: 1}, {Split: "development", Answerable: true, Recall: &one}}
	metrics := aggregate(results, "holdout")
	if metrics.Questions != 3 || metrics.Answerable != 2 || metrics.Unknown != 1 || metrics.MeanRecall == nil || *metrics.MeanRecall != .5 || metrics.WrongUnknownEvidence != 1 || metrics.ThresholdPassed {
		t.Fatalf("bad aggregate %#v", metrics)
	}
	empty := aggregate(nil, "holdout")
	if empty.MeanRecall != nil || empty.ThresholdPassed {
		t.Fatal("zero denominator fabricated success")
	}
}
