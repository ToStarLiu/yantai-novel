package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSemanticEmbeddingRerankAndVisibilityAfterVersionChange(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter := f.chapter(t, "磨坊", "林川暂住北岸磨坊。")
	f.apply(t, f.proposal(t, chapter, []MemoryChange{}))
	embeddingCalls, rerankCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/embeddings" {
			embeddingCalls++
			var texts []string
			_ = json.Unmarshal(body["input"], &texts)
			data := []any{}
			for i := range texts {
				data = append(data, map[string]any{"index": i, "embedding": []float64{1, 0, 0}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
			return
		}
		if r.URL.Path == "/rerank" {
			rerankCalls++
			fmt.Fprint(w, `{"results":[{"index":0,"relevance_score":0.98}]}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	embedding, err := f.s.SaveEmbeddingConfig(ctx, EmbeddingConfig{Name: "mock embedding", BaseURL: server.URL, Model: "e-v1", Dimension: 3, Kind: "embedding", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rerank, err := f.s.SaveEmbeddingConfig(ctx, EmbeddingConfig{Name: "mock rerank", BaseURL: server.URL, Model: "r-v1", Dimension: 1, Kind: "rerank", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	n, err := f.s.RebuildEmbeddings(ctx, f.p.ID, embedding.ID)
	if err != nil || n != 1 {
		t.Fatalf("embedding rebuild %d %v", n, err)
	}
	result, err := f.s.SemanticSearch(ctx, SemanticQuery{Query: Query{ProjectID: f.p.ID, Text: "最初的旧作坊", Purpose: "AuthorReview", Limit: 10}, Mode: "hybrid", EmbeddingProviderID: embedding.ID, RerankProviderID: rerank.ID})
	if err != nil || len(result) != 1 || result[0].RevisionID != chapter.FinalRevisionID || embeddingCalls != 2 || rerankCalls != 1 {
		t.Fatalf("semantic path %#v %v calls=%d/%d", result, err, embeddingCalls, rerankCalls)
	}
	if _, err = f.s.DB.Exec(ctx, "UPDATE wb_embeddings SET dimension=2 WHERE config_id=$1", embedding.ID); err != nil {
		t.Fatal(err)
	}
	result, err = f.s.SemanticSearch(ctx, SemanticQuery{Query: Query{ProjectID: f.p.ID, Text: "最初的旧作坊"}, Mode: "vector", EmbeddingProviderID: embedding.ID})
	if err != nil || len(result) != 0 {
		t.Fatalf("mixed vector dimensions leaked %#v %v", result, err)
	}
	if _, err = f.s.RebuildEmbeddings(ctx, f.p.ID, embedding.ID); err != nil {
		t.Fatal(err)
	}
	changed, err := f.s.SaveRevision(ctx, chapter.ID, SaveInput{ExpectedCurrentRevisionID: chapter.CurrentRevisionID, Document: DocumentFromText("林川已离开旧地。")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Finalize(ctx, chapter.ID, changed.ID); err != nil {
		t.Fatal(err)
	}
	result, err = f.s.SemanticSearch(ctx, SemanticQuery{Query: Query{ProjectID: f.p.ID, Text: "磨坊"}, Mode: "vector", EmbeddingProviderID: embedding.ID})
	if err != nil || len(result) != 0 {
		t.Fatalf("invalidated semantic source leaked %#v %v", result, err)
	}
}

func TestSemanticRejectsBadVectorsRerankAndSummarySources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"index":0,"embedding":[1,2]}]}`)
	}))
	defer server.Close()
	if _, err := embedTexts(context.Background(), EmbeddingConfig{BaseURL: server.URL, Dimension: 3, Model: "m"}, []string{"x"}); err == nil {
		t.Fatal("wrong dimension accepted")
	}
	if _, err := cosine([]float64{1, 0}, []float64{1}); err == nil {
		t.Fatal("dimension mismatch accepted")
	}
	for _, magnitude := range []float64{1e308, 1e-308} {
		score, err := cosine([]float64{magnitude, magnitude}, []float64{magnitude, magnitude})
		if err != nil || math.IsNaN(score) || math.Abs(score-1) > 1e-12 {
			t.Fatal("finite cosine overflow/underflow", magnitude, score, err)
		}
	}
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "第一章", "林川左臂受伤，停在北岸磨坊。")
	f.apply(t, f.proposal(t, c, []MemoryChange{}))
	summary, err := f.s.CompressContext(ctx, Query{ProjectID: f.p.ID, Text: "磨坊", Limit: 10}, 500)
	if err != nil || len(summary.Sources) != 1 || !summary.Derived || !strings.Contains(summary.Summary, "磨坊") {
		t.Fatalf("summary not sourced %#v %v", summary, err)
	}
	if _, err = f.s.GetContextSummary(ctx, summary.ID); err != nil {
		t.Fatal(err)
	}
	forged := append([]Citation(nil), summary.Sources...)
	forged[0].ChapterOrder = 0
	if _, err = f.s.SaveContextSummary(ctx, f.p.ID, "伪造前文范围", forged); err == nil {
		t.Fatal("summary accepted forged source position")
	}
	foreign := summary.Sources
	foreign[0].ProjectID = "another-work"
	if _, err = f.s.SaveContextSummary(ctx, f.p.ID, "虚构摘要", foreign); err == nil {
		t.Fatal("cross-work summary evidence accepted")
	}
	revision, err := f.s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("旧稿已替换。")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Finalize(ctx, c.ID, revision.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.GetContextSummary(ctx, summary.ID); !IsConflict(err) {
		t.Fatalf("old-source summary remained usable %v", err)
	}
}

func TestSemanticRechecksSourcesAfterRemoteEmbeddingCall(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	chapter := f.chapter(t, "第一章", "铜印留在北岸磨坊。")
	f.apply(t, f.proposal(t, chapter, []MemoryChange{}))
	invalidate := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if invalidate {
			revision, err := f.s.SaveRevision(ctx, chapter.ID, SaveInput{ExpectedCurrentRevisionID: chapter.CurrentRevisionID, Document: DocumentFromText("铜印已经不在此地。")})
			if err != nil {
				t.Error(err)
			} else if _, err = f.s.Finalize(ctx, chapter.ID, revision.ID); err != nil {
				t.Error(err)
			}
		}
		data := []any{}
		for i := range body.Input {
			data = append(data, map[string]any{"index": i, "embedding": []float64{1, 0}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()
	config, err := f.s.SaveEmbeddingConfig(ctx, EmbeddingConfig{Name: "变化中语义", BaseURL: server.URL, Model: "embedding", Dimension: 2, Kind: "embedding", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RebuildEmbeddings(ctx, f.p.ID, config.ID); err != nil {
		t.Fatal(err)
	}
	invalidate = true
	results, err := f.s.SemanticSearch(ctx, SemanticQuery{Query: Query{ProjectID: f.p.ID, Text: "磨坊"}, Mode: "hybrid", EmbeddingProviderID: config.ID})
	if err != nil || len(results) != 0 {
		t.Fatalf("remote delay resurrected invalid source %#v %v", results, err)
	}
}
