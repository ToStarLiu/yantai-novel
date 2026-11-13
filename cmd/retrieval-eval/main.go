package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"novel/internal/job"
	"novel/internal/workbench"
	"novel/migrations"
)

type Chapter struct {
	ID, Title, Location, Event, Password, Holder, Injury, Text string
	Quotes                                                     map[string]string
}
type Question struct {
	ID              string `json:"id"`
	Question        string `json:"question"`
	Answerable      bool   `json:"answerable"`
	Purpose         string `json:"purpose"`
	CharacterID     string `json:"characterId"`
	BeforeChapterID string `json:"beforeChapterId"`
	Tag             string `json:"tag"`
	Split           string `json:"split"`
}
type Relation struct {
	QuestionID string `json:"questionId"`
	Evidence   []struct {
		Source    string `json:"source"`
		ChapterID string `json:"chapterId"`
		Quote     string `json:"quote"`
	} `json:"evidence"`
}
type Result struct {
	ID                string               `json:"id"`
	Split             string               `json:"split"`
	Tag               string               `json:"tag"`
	Question          string               `json:"question"`
	Answerable        bool                 `json:"answerable"`
	Recall            *float64             `json:"recallAt10"`
	WrongEvidence     int                  `json:"wrongEvidence"`
	Retrieved         []workbench.Citation `json:"retrieved"`
	ExpectedSourceIDs []string             `json:"expectedSourceRevisionIds"`
}
type Metrics struct {
	Questions            int      `json:"questions"`
	Answerable           int      `json:"answerable"`
	Unknown              int      `json:"unknown"`
	MeanRecall           *float64 `json:"meanRecallAt10"`
	WrongUnknownEvidence int      `json:"wrongUnknownEvidence"`
	ThresholdPassed      bool     `json:"thresholdPassed"`
}
type Report struct {
	FixtureVersion string         `json:"fixtureVersion"`
	FixtureHash    string         `json:"fixtureHash"`
	Mode           string         `json:"mode"`
	StartedAt      time.Time      `json:"startedAt"`
	FinishedAt     time.Time      `json:"finishedAt"`
	Results        []Result       `json:"results"`
	Development    Metrics        `json:"development"`
	Holdout        Metrics        `json:"holdout"`
	Isolation      map[string]int `json:"isolationViolations"`
	Notes          []string       `json:"notes"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	fixture := flag.String("fixture", "evaluation/fixtures/retrieval-60", "frozen fixture directory")
	output := flag.String("output", "evaluation/results/retrieval-keyword", "new output directory")
	flag.Parse()
	var story struct {
		FixtureVersion string
		Chapters       []Chapter
	}
	var questions struct{ Questions []Question }
	var labels struct{ Qrels []Relation }
	var fixedSplit struct {
		Development []string `json:"development"`
		Holdout     []string `json:"holdout"`
	}
	var hashMaterial []byte
	for _, file := range []struct {
		Name  string
		Value any
	}{{"story.json", &story}, {"questions.json", &questions}, {"qrels.json", &labels}} {
		raw, err := os.ReadFile(filepath.Join(*fixture, file.Name))
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, file.Value); err != nil {
			return err
		}
		hashMaterial = append(hashMaterial, raw...)
	}
	splits, err := os.ReadFile(filepath.Join(*fixture, "split.json"))
	if err != nil {
		return err
	}
	hashMaterial = append(hashMaterial, splits...)
	if err = json.Unmarshal(splits, &fixedSplit); err != nil {
		return err
	}
	if len(fixedSplit.Development) != 40 || len(fixedSplit.Holdout) != 20 {
		return fmt.Errorf("frozen split must have 40 development and 20 holdout IDs")
	}
	splitMap := map[string]string{}
	for _, id := range fixedSplit.Development {
		if splitMap[id] != "" {
			return fmt.Errorf("duplicate split ID")
		}
		splitMap[id] = "development"
	}
	for _, id := range fixedSplit.Holdout {
		if splitMap[id] != "" {
			return fmt.Errorf("duplicate split ID")
		}
		splitMap[id] = "holdout"
	}
	if len(story.Chapters) != 20 || len(questions.Questions) != 60 || len(labels.Qrels) != 60 {
		return fmt.Errorf("fixture must contain 20 chapters and 60 independently labelled questions")
	}
	labelMap := map[string]Relation{}
	for _, label := range labels.Qrels {
		if label.QuestionID == "" || labelMap[label.QuestionID].QuestionID != "" {
			return fmt.Errorf("duplicate or missing qrels ID")
		}
		labelMap[label.QuestionID] = label
	}
	chapterByID := map[string]Chapter{}
	for _, c := range story.Chapters {
		chapterByID[c.ID] = c
	}
	for _, q := range questions.Questions {
		if splitMap[q.ID] != q.Split {
			return fmt.Errorf("question %s does not match frozen split", q.ID)
		}
		label, ok := labelMap[q.ID]
		if !ok || q.Answerable != (len(label.Evidence) > 0) {
			return fmt.Errorf("invalid qrels for %s", q.ID)
		}
		for _, e := range label.Evidence {
			c, ok := chapterByID[e.ChapterID]
			if !ok || e.Source != "chapters/"+e.ChapterID || !strings.Contains(c.Text, e.Quote) {
				return fmt.Errorf("invalid source quote for %s", q.ID)
			}
		}
	}
	if _, err = os.Stat(*output); err == nil {
		return fmt.Errorf("output exists; choose a new directory")
	} else if !os.IsNotExist(err) {
		return err
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		return fmt.Errorf("TEST_DATABASE_URL or DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	defer admin.Close()
	schema := "test_retrieval_" + job.NewID()
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		return err
	}
	defer func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE") }()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return err
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db, "up"); err != nil {
		return err
	}
	service := &workbench.Service{DB: db}
	p, err := service.CreateProject(ctx, "检索受控故事", "原创20短章；无模型调用", "")
	if err != nil {
		return err
	}
	entities := map[string]string{}
	for _, entity := range []workbench.Setting{{Kind: "character", Name: "林川"}, {Kind: "character", Name: "沈月"}, {Kind: "world", Name: "铜印"}} {
		setting, e := service.SaveSetting(ctx, p.ID, entity)
		if e != nil {
			return e
		}
		switch entity.Name {
		case "林川":
			entities["lin-chuan"] = setting.ID
		case "沈月":
			entities["shen-yue"] = setting.ID
		case "铜印":
			entities["bronze-seal"] = setting.ID
		}
	}
	chapterIDs := map[string]string{}
	revisionIDs := map[string]string{}
	for index, input := range story.Chapters {
		chapter, e := service.CreateChapter(ctx, p.ID, input.Title, "")
		if e != nil {
			return e
		}
		chapterIDs[input.ID] = chapter.ID
		revision, e := service.SaveRevision(ctx, chapter.ID, workbench.SaveInput{ExpectedCurrentRevisionID: chapter.CurrentRevisionID, Document: workbench.DocumentFromText(input.Text), Source: "author"})
		if e != nil {
			return e
		}
		revisionIDs[input.ID] = revision.ID
		if _, e = service.Finalize(ctx, chapter.ID, revision.ID); e != nil {
			return e
		}
		memory, e := service.MemoryInput(ctx, chapter.ID)
		if e != nil {
			return e
		}
		evidence := func(key string) workbench.Evidence {
			return workbench.Evidence{RevisionID: revision.ID, PlainTextHash: revision.Hash, Quote: input.Quotes[key]}
		}
		changes := []workbench.MemoryChange{
			{Kind: "Location", Subject: entities["lin-chuan"], Predicate: "location", Object: input.Location, TruthStatus: "True", Evidence: evidence("location")},
			{Kind: "Injury", Subject: entities["lin-chuan"], Predicate: "injury", Object: input.Injury, TruthStatus: "True", Evidence: evidence("injury")},
			{Kind: "Ownership", Subject: entities["bronze-seal"], Predicate: "holder", Object: entities[input.Holder], TruthStatus: "True", Evidence: evidence("ownership")},
			{Kind: "Knowledge", Subject: entities["bronze-seal"], Predicate: "password-" + input.ID, Object: input.Password, CharacterID: entities["shen-yue"], TruthStatus: "True", Evidence: evidence("knowledge")},
			{Kind: "Foreshadow", Subject: entities["bronze-seal"], Predicate: "unresolved-" + input.ID, Object: input.Event, TruthStatus: "True", Evidence: evidence("foreshadow")},
		}
		day := index + 1
		for i := range changes {
			changes[i].StoryTime = &day
		}
		proposal, e := service.CreateProposal(ctx, chapter.ID, revision.ID, memory.BaseStateID, memory.Epoch, changes)
		if e != nil {
			return e
		}
		decisions := []workbench.MemoryDecision{}
		for _, change := range proposal.Changes {
			decisions = append(decisions, workbench.MemoryDecision{ID: change.ID, Decision: "Accept"})
		}
		if _, e = service.DecideProposal(ctx, proposal.ID, decisions); e != nil {
			return e
		}
		if _, e = service.ApplyProposal(ctx, proposal.ID, memory.BaseStateID, revision.ID); e != nil {
			return e
		}
	}
	report := Report{FixtureVersion: story.FixtureVersion, FixtureHash: job.Hash(hashMaterial), Mode: "keyword", StartedAt: time.Now().UTC(), Results: []Result{}, Isolation: map[string]int{"crossProject": 0, "oldRevision": 0, "future": 0, "characterKnowledge": 0}, Notes: []string{"Fixture and 40/20 split frozen before the first execution; no model calls or embeddings.", "Twenty short controlled chapters are engineering fixtures, not evidence of literary long-form quality.", "Synonym-only questions are deliberately retained; word-overlap success does not establish semantic retrieval quality.", "Recall@10 is macro-averaged over answerable questions; unknown questions are reported separately."}}
	for _, q := range questions.Questions {
		result := Result{ID: q.ID, Split: q.Split, Tag: q.Tag, Question: q.Question, Answerable: q.Answerable, ExpectedSourceIDs: []string{}}
		for _, e := range labelMap[q.ID].Evidence {
			result.ExpectedSourceIDs = append(result.ExpectedSourceIDs, revisionIDs[e.ChapterID])
		}
		result.Retrieved, err = service.Search(ctx, workbench.Query{ProjectID: p.ID, Text: q.Question, BeforeChapterID: chapterIDs[q.BeforeChapterID], CharacterID: entities[q.CharacterID], Purpose: q.Purpose, Limit: 10})
		if err != nil {
			return err
		}
		if q.Answerable {
			hits := 0
			for _, expected := range labelMap[q.ID].Evidence {
				for _, citation := range result.Retrieved {
					// Character projections return only acquired values, keeping
					// surrounding narrator secrets outside the result. Validate
					// source identity and evidence containment in that direction.
					quoteCovered := strings.Contains(citation.Content, expected.Quote)
					if q.Purpose == "CharacterKnowledge" {
						quoteCovered = citation.Content != "" && strings.Contains(expected.Quote, citation.Content)
					}
					if citation.RevisionID == revisionIDs[expected.ChapterID] && quoteCovered {
						hits++
						break
					}
				}
			}
			recall := float64(hits) / float64(len(labelMap[q.ID].Evidence))
			result.Recall = &recall
		} else {
			result.WrongEvidence = len(result.Retrieved)
			if q.Tag == "future-invisible" {
				report.Isolation["future"] += result.WrongEvidence
			}
			if q.Tag == "character-invisible" {
				report.Isolation["characterKnowledge"] += result.WrongEvidence
			}
		}
		report.Results = append(report.Results, result)
	}
	// Controlled additional checks explicitly exercise wrong-project and old
	// revision visibility, rather than inferring isolation from aggregate scores.
	foreign, err := service.CreateProject(ctx, "隔离作品", "", "")
	if err != nil {
		return err
	}
	leak, err := service.Search(ctx, workbench.Query{ProjectID: foreign.ID, Text: story.Chapters[0].Location, Purpose: "AuthorReview", Limit: 10})
	if err != nil {
		return err
	}
	report.Isolation["crossProject"] = len(leak)
	first, err := service.GetChapter(ctx, chapterIDs["01"])
	if err != nil {
		return err
	}
	changed, err := service.SaveRevision(ctx, first.ID, workbench.SaveInput{ExpectedCurrentRevisionID: first.CurrentRevisionID, Document: workbench.DocumentFromText("此章已换版，不包含原始地点与异常。")})
	if err != nil {
		return err
	}
	if _, err = service.Finalize(ctx, first.ID, changed.ID); err != nil {
		return err
	}
	after, err := service.Search(ctx, workbench.Query{ProjectID: p.ID, Text: story.Chapters[0].Location, Purpose: "AuthorReview", Limit: 10})
	if err != nil {
		return err
	}
	for _, citation := range after {
		if citation.RevisionID == revisionIDs["01"] {
			report.Isolation["oldRevision"]++
		}
	}
	report.Development = aggregate(report.Results, "development")
	report.Holdout = aggregate(report.Results, "holdout")
	report.FinishedAt = time.Now().UTC()
	if err = os.MkdirAll(*output, 0700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(filepath.Join(*output, "results.json"), raw, 0600); err != nil {
		return err
	}
	summary := fmt.Sprintf("# 关键词检索基线\n\n样例 %s，固定40开发/20保留，20短章，60问题。\n\n| 分组 | 有答案 | 无答案 | Recall@10 | 无答案错误返回 | 门槛 |\n|---|---:|---:|---:|---:|---|\n| development | %d | %d | %s | %d | %t |\n| holdout | %d | %d | %s | %d | %t |\n\n隔离违规：%v。所有逐题来源、引用、分组及失败见 results.json。\n\n专用随机schema在程序退出时清理，没有改变正常作品，没有调用模型或Embedding。纯同义问题保留失败；不要据字面重合结果宣称语义或长篇质量成立。\n", report.FixtureVersion, report.Development.Answerable, report.Development.Unknown, percent(report.Development.MeanRecall), report.Development.WrongUnknownEvidence, report.Development.ThresholdPassed, report.Holdout.Answerable, report.Holdout.Unknown, percent(report.Holdout.MeanRecall), report.Holdout.WrongUnknownEvidence, report.Holdout.ThresholdPassed, report.Isolation)
	if err = os.WriteFile(filepath.Join(*output, "summary.md"), []byte(summary), 0600); err != nil {
		return err
	}
	fmt.Print(summary)
	return nil
}
func aggregate(results []Result, split string) Metrics {
	m := Metrics{}
	sum := 0.0
	for _, r := range results {
		if r.Split != split {
			continue
		}
		m.Questions++
		if r.Answerable {
			m.Answerable++
			sum += *r.Recall
		} else {
			m.Unknown++
			m.WrongUnknownEvidence += r.WrongEvidence
		}
	}
	if m.Answerable > 0 {
		mean := sum / float64(m.Answerable)
		m.MeanRecall = &mean
		m.ThresholdPassed = mean >= .9 && m.WrongUnknownEvidence == 0
	}
	return m
}
func percent(n *float64) string {
	if n == nil {
		return "null"
	}
	return fmt.Sprintf("%.2f%%", *n*100)
}
