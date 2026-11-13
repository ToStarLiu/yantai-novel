package workbench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMediaQueuedAttemptAndProjectScopedContent(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-media" {
			t.Error("auth missing")
		}
		if r.URL.Path != "/v1/audio/speech" {
			t.Error(r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !strings.Contains(body["input"].(string), "左臂") {
			t.Error("chapter input not frozen")
		}
		var status string
		if e := s.DB.QueryRow(ctx, "SELECT status FROM wb_media LIMIT 1").Scan(&status); e != nil || status != "Running" {
			t.Error("external work before durable status")
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3test-audio"))
	}))
	defer provider.Close()
	s.MediaConfig = MediaConfig{BaseURL: provider.URL + "/v1", APIKey: "test-media", SpeechModel: "tts-model"}
	a, e := s.SubmitMedia(ctx, c.ProjectID, MediaRequest{Kind: "tts", ChapterID: c.ID})
	if e != nil {
		t.Fatal(e)
	}
	if calls != 0 {
		t.Fatal("request not queued")
	}
	if e = s.AdvanceMedia(ctx); e != nil {
		t.Fatal(e)
	}
	done, e := s.GetMedia(ctx, c.ProjectID, a.ID)
	if e != nil || done.Status != "Succeeded" {
		t.Fatalf("media %+v %v", done, e)
	}
	content, mime, e := s.MediaContent(ctx, c.ProjectID, a.ID)
	if e != nil || mime != "audio/mpeg" || string(content) != "ID3test-audio" {
		t.Fatal("audio not durable", e)
	}
	other, e := s.CreateProject(ctx, "其他", "", "")
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.MediaContent(ctx, other.ID, a.ID); e == nil {
		t.Fatal("cross-project asset leaked")
	}
	if e = s.AdvanceMedia(ctx); e != nil || calls != 1 {
		t.Fatal("media repeated", e)
	}
}
func TestMediaFailureUnknownNeverAutomaticallyRetried(t *testing.T) {
	s, c := runtimeFixture(t)
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer provider.Close()
	s.MediaConfig = MediaConfig{BaseURL: provider.URL, APIKey: "test", ImageModel: "image-model"}
	a, e := s.SubmitMedia(context.Background(), c.ProjectID, MediaRequest{Kind: "cover", Prompt: "雨夜旧桥"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AdvanceMedia(context.Background()); e != nil {
		t.Fatal(e)
	}
	done, _ := s.GetMedia(context.Background(), c.ProjectID, a.ID)
	if done.Status != "NeedsReconciliation" {
		t.Fatal(done.Status)
	}
	_ = s.AdvanceMedia(context.Background())
	if calls != 1 {
		t.Fatal("failed media retried")
	}
}
