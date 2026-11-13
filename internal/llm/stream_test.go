package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResponsesStreamEmitsBeforeCompletionAndPreservesUsage(t *testing.T) {
	gotFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["stream"] != true || body["store"] != false || body["max_output_tokens"] != float64(300) || r.Header.Get("x-test-header") != "actor" {
			t.Errorf("incorrect stream request %#v", body)
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("Accept") != "text/event-stream" {
			t.Error("missing stream headers")
		}
		text, ok := body["text"].(map[string]any)
		if !ok || text["format"] == nil {
			t.Error("structured schema lost")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"林川\"}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-gotFirst:
		case <-time.After(time.Second):
			t.Error("delta buffered until completion")
		}
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"受伤。\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-stream\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"林川受伤。\"}]}],\"usage\":{\"input_tokens\":30,\"output_tokens\":4,\"input_tokens_details\":{\"cached_tokens\":8}}}}\n\n")
	}))
	defer server.Close()
	p, err := NewCompatible(Config{BaseURL: server.URL, APIKey: "test-secret", WireAPI: "responses", Headers: map[string]string{"x-test-header": "actor"}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	req := testRequest()
	req.ResponseSchema = []byte(`{"type":"object","additionalProperties":false,"properties":{}}`)
	deltas := []string{}
	result, err := p.Stream(context.Background(), req, func(d Delta) error {
		deltas = append(deltas, d.Text)
		if len(deltas) == 1 {
			close(gotFirst)
		}
		return nil
	})
	if err != nil || result.Text != "林川受伤。" || strings.Join(deltas, "") != result.Text || len(deltas) != 2 {
		t.Fatalf("bad stream %#v %v %v", result, deltas, err)
	}
	if result.ProviderRequestID != "resp-stream" || !result.Usage.Reported || result.Usage.CachedInputTokens != 8 || !strings.Contains(string(result.Raw), "response.completed") {
		t.Fatalf("metadata lost %#v", result)
	}
}

func TestChatStreamUsageFinishAndUnknownEOF(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(fmt.Sprint(complete), func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				options, _ := body["stream_options"].(map[string]any)
				if options["include_usage"] != true || body["store"] != false {
					t.Error("usage/store request missing")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"chat-stream\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"铜印\"},\"finish_reason\":null}]}\n\n")
				if complete {
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			result, err := testProvider(t, server).Stream(context.Background(), testRequest(), func(Delta) error { return nil })
			if result.Text != "铜印" || calls.Load() != 1 {
				t.Fatalf("partial text/request count lost %#v", result)
			}
			if complete {
				if err != nil || !result.Usage.Reported || result.Usage.InputTokens != 10 {
					t.Fatalf("successful stream %#v %v", result, err)
				}
			} else if err == nil || FailureDisposition(err, true) != NeedsReconciliation {
				t.Fatalf("EOF must be unknown %v", err)
			}
		})
	}
}

func TestStreamEmitFailureStopsWithoutRetry(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"第一段\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"第二段\"}}]}\n\n")
	}))
	defer server.Close()
	emits := 0
	result, err := testProvider(t, server).Stream(context.Background(), testRequest(), func(Delta) error { emits++; return errors.New("database password must not leak") })
	if err == nil || FailureDisposition(err, true) != NeedsReconciliation || emits != 1 || calls.Load() != 1 || result.Text != "第一段" {
		t.Fatalf("emit failure %#v emits=%d err=%v", result, emits, err)
	}
	if strings.Contains(err.Error(), "password") {
		t.Fatal("consumer credentials leaked")
	}
}

func TestStreamIncompleteMalformedTimeoutAndSizeLimit(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		delay      time.Duration
	}{
		{name: "incomplete", body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"部分\"}\n\ndata: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"incomplete\",\"status\":\"incomplete\",\"output\":[],\"usage\":{\"input_tokens\":9,\"output_tokens\":1}}}\n\n"},
		{name: "malformed", body: "data: not-json\n\n"},
		{name: "oversize", body: "data: " + strings.Repeat("x", 300) + "\n\n", limit: 64},
		{name: "timeout", body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"部分\"}\n\n", delay: 200 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.body)
				w.(http.Flusher).Flush()
				if tc.delay > 0 {
					select {
					case <-r.Context().Done():
					case <-time.After(tc.delay):
					}
				}
			}))
			defer server.Close()
			timeout := time.Second
			if tc.delay > 0 {
				timeout = 30 * time.Millisecond
			}
			p, err := NewCompatible(Config{BaseURL: server.URL, APIKey: "test-secret", WireAPI: "responses", Timeout: timeout, MaxResponseBytes: tc.limit})
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.Stream(context.Background(), testRequest(), nil)
			if err == nil || FailureDisposition(err, true) != NeedsReconciliation || calls.Load() != 1 {
				t.Fatalf("must fail unknown without retry %#v %v", result, err)
			}
			if tc.limit > 0 && int64(len(result.Raw)) > tc.limit {
				t.Fatal("raw buffer exceeded cap")
			}
			if tc.name == "incomplete" && (!result.Usage.Reported || result.Text != "部分") {
				t.Fatalf("partial metadata lost %#v", result)
			}
		})
	}
}

func TestStreamJSONFallbackUsesSameResponseWithoutReplay(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"完整候选"}}]}`)
	}))
	defer server.Close()
	emits := 0
	result, err := testProvider(t, server).Stream(context.Background(), testRequest(), func(d Delta) error {
		emits++
		if d.Text != "完整候选" {
			t.Error(d)
		}
		return nil
	})
	if err != nil || calls.Load() != 1 || emits != 1 || result.Text != "完整候选" {
		t.Fatalf("fallback retried or synthesized tokens %#v %v", result, err)
	}
}
