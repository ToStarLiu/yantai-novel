package workbench

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"novel/internal/job"
)

type researchTransport func(*http.Request) (*http.Response, error)

func (f researchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func researchResponse(r *http.Request, status int, contentType, text string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(text)), Request: r}
}

func TestResearchFetchStoresRealSourceEvidenceAndPartialErrors(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	ctx := context.Background()
	calls := 0
	client := &http.Client{Transport: researchTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" {
			t.Fatal("research method not GET")
		}
		if r.URL.Path == "/missing" {
			return researchResponse(r, 404, "text/html", "not found"), nil
		}
		return researchResponse(r, 200, "text/html; charset=utf-8", `<html><head><title>原创榜单资料</title><style>.x{display:none}</style></head><body><h1>公开题材观察</h1><p>玄河调查故事登上读者推荐。</p><p>人物目标与失败代价是分析维度。</p><a href="/book/1">故事一</a><script>privateScriptToken()</script></body></html>`), nil
	})}
	report, err := s.ResearchWithClient(ctx, p.ID, "观察公开题材与结构", []string{"https://8.8.8.8/ranking", "https://8.8.8.8/missing"}, client)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || report.Status != "Partial" || report.Sources[0].Title != "原创榜单资料" || report.Sources[0].ReferenceID == "" || report.Sources[1].Error == "" || len(report.Sources[0].Links) != 1 {
		t.Fatalf("research %#v", report)
	}
	if strings.Contains(report.Sources[0].Text, "privateScriptToken") || !strings.Contains(report.Sources[0].Excerpt, "玄河调查") {
		t.Fatal("HTML extraction lost evidence or retained scripts")
	}
	list, err := s.ListResearch(ctx, p.ID)
	if err != nil || len(list) != 1 || list[0].Sources[0].URL != "https://8.8.8.8/ranking" {
		t.Fatal("research sources not persisted")
	}
	references, err := s.StudioReferences(ctx, p.ID)
	if err != nil || len(references) != 1 || references[0].Analysis.ParagraphCount == 0 {
		t.Fatal("fetched source not stored in isolated analyzed library")
	}
	canonical, err := s.Search(ctx, Query{ProjectID: p.ID, Purpose: "AuthorReview", Text: "玄河调查"})
	if err != nil || len(canonical) != 0 {
		t.Fatal("external research leaked into canonical index")
	}
	other, err := s.CreateProject(ctx, "其他作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.ListResearch(ctx, other.ID)
	if err != nil || len(foreign) != 0 {
		t.Fatal("research crossed project")
	}
}

func TestResearchRejectsPrivateRedirectBinaryAndLargeSources(t *testing.T) {
	ctx := context.Background()
	calls := 0
	client := &http.Client{Transport: researchTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.URL.Path {
		case "/redirect":
			resp := researchResponse(r, 302, "text/html", "")
			resp.Header.Set("Location", "http://127.0.0.1/secret")
			return resp, nil
		case "/binary":
			return researchResponse(r, 200, "image/png", "png bytes"), nil
		default:
			return researchResponse(r, 200, "text/plain", strings.Repeat("x", researchMaxBytes+1)), nil
		}
	})}
	for _, raw := range []string{"file:///private", "http://localhost/x", "http://127.0.0.1/x", "http://10.0.0.1/x", "http://169.254.169.254/latest", "http://[::1]/x", "https://user:password@example.com/x", "https://8.8.8.8:8080/x"} {
		if _, err := fetchResearch(ctx, guardedResearchClient(client), raw); err == nil {
			t.Fatalf("unsafe URL accepted %s", raw)
		}
	}
	if calls != 0 {
		t.Fatal("unsafe URL reached network")
	}
	for _, path := range []string{"redirect", "binary", "large"} {
		before := calls
		source, err := fetchResearch(ctx, guardedResearchClient(client), "https://8.8.8.8/"+path)
		if err == nil || source.ReferenceID != "" {
			t.Fatalf("invalid source accepted %s", path)
		}
		if calls != before+1 {
			t.Fatal("redirect followed private host or request retried")
		}
	}
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "192.0.2.1"} {
		if _, err := researchResolve(ctx, host); err == nil {
			t.Fatalf("private/reserved dial target accepted %s", host)
		}
	}
}

func TestResearchSearchUsesConfiguredServiceAndPreservesLinks(t *testing.T) {
	t.Setenv("RESEARCH_SEARCH_URL", "")
	if _, err := ResearchSearch(context.Background(), "悬疑榜单"); err == nil {
		t.Fatal("unconfigured web search pretended available")
	}
	t.Setenv("RESEARCH_SEARCH_URL", "https://8.8.8.8/search")
	t.Setenv("RESEARCH_SEARCH_API_KEY", "test-only-key")
	client := &http.Client{Transport: researchTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("q") != "悬疑榜单" || r.Header.Get("Authorization") != "Bearer test-only-key" {
			t.Fatal("configured search request missing query/auth")
		}
		return researchResponse(r, 200, "application/json", `{"results":[{"title":"公开榜单","url":"https://example.com/ranking","snippet":"公开内容"},{"title":"内部地址","url":"http://127.0.0.1/secret","snippet":"拒绝"}]}`), nil
	})}
	hits, err := researchSearchWithClient(context.Background(), "悬疑榜单", client)
	if err != nil || len(hits) != 1 || hits[0].URL != "https://example.com/ranking" {
		t.Fatalf("search %v %#v", err, hits)
	}
}

func TestBookVolumePlanAdoptionValidatesSourceAndKeepsFacts(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	brief, err := s.SaveStudioBrief(ctx, p.ID, StudioWrite[StoryBrief]{Data: StoryBrief{CoreConflict: "寻找真相", Volumes: []StoryVolume{{ID: "v1", Title: "初入谜局", ChapterCount: 10}}}})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.StudioPromptContext(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	makeRun := func(scope, volumeID string) string {
		t.Helper()
		id := job.NewID()
		input := map[string]any{"projectId": p.ID, "chapterId": c.ID, "revisionId": c.CurrentRevisionID, "epoch": detail.Project.Epoch, "stateVersionId": "", "blueprintVersion": c.BlueprintVersion, "studioVersions": snapshot.Versions, "scope": scope, "volumeId": volumeID}
		raw, _ := json.Marshal(input)
		_, e := s.DB.Exec(ctx, "INSERT INTO wb_runs(id,project_id,chapter_id,type,status,input,output,intent_id,input_hash,epoch,revision_id) VALUES($1,$2,$3,'planning','Succeeded',$4,'{\"text\":\"规划候选\"}',$5,'hash',$6,$7)", id, p.ID, c.ID, raw, job.NewID(), detail.Project.Epoch, c.CurrentRevisionID)
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	bookRun := makeRun("book", "")
	saved, err := s.ApplyStudioPlan(ctx, p.ID, ApplyPlanInput{ExpectedVersion: brief.Version, Scope: "book", Text: "全书从误信开始，经过追索，最终承担真相的代价。", RunID: bookRun})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Data.PlotPlan == "" || saved.Version != 2 {
		t.Fatal("book plan not adopted")
	}
	if _, err = s.ApplyStudioPlan(ctx, p.ID, ApplyPlanInput{ExpectedVersion: 2, Scope: "book", Text: "过期结果", RunID: bookRun}); !IsConflict(err) {
		t.Fatal("old Studio plan candidate reapplied")
	}
	snapshot, err = s.StudioPromptContext(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	volumeRun := makeRun("volume", "v1")
	saved, err = s.ApplyStudioPlan(ctx, p.ID, ApplyPlanInput{ExpectedVersion: 2, Scope: "volume", VolumeID: "v1", Text: "第一卷获得铜印并建立不稳固的同盟。", RunID: volumeRun})
	if err != nil || saved.Data.Volumes[0].Summary == "" {
		t.Fatalf("volume plan %v %#v", err, saved)
	}
	state, err := s.State(ctx, p.ID)
	if err != nil || len(state.Facts) != 0 {
		t.Fatal("future planning became canon")
	}
}

func TestReferenceOverlapReturnsExactSourceWithoutMixingFacts(t *testing.T) {
	s := newCore(t)
	p, c := setupCore(t, s)
	ctx := context.Background()
	quote := "夜雨沿着旧磨坊的窗框不断落下，铜印静静地留在桌角等待来人。"
	if _, err := s.AddStudioReference(ctx, p.ID, "原创参考", quote, ""); err != nil {
		t.Fatal(err)
	}
	r := saveCore(t, s, c, "新段落。"+quote+"结尾。")
	overlaps, err := s.ReferenceOverlap(ctx, p.ID, r.ID)
	if err != nil || len(overlaps) == 0 || !strings.Contains(r.PlainText, overlaps[0].Quote) || overlaps[0].Length < 20 {
		t.Fatalf("overlap %v %#v", err, overlaps)
	}
}
