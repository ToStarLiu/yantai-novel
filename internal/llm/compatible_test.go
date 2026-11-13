package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testRequest() Request {
	return Request{Model: "test-model", Messages: []Message{{Role: "user", Content: "请审稿"}}, MaxOutputTokens: 300}
}

func testProvider(t *testing.T, server *httptest.Server) *Compatible {
	t.Helper()
	provider, err := NewCompatible(Config{BaseURL: server.URL + "/v1", APIKey: "test-secret", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestCompatibleSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing authentication")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["model"] != "test-model" || payload["max_tokens"] != float64(300) {
			t.Errorf("unexpected payload: %v", payload)
		}
		format, ok := payload["response_format"].(map[string]any)
		if !ok || format["type"] != "json_schema" {
			t.Error("missing structured output format")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"provider-123","choices":[{"message":{"content":"正文"}}],"usage":{"prompt_tokens":20,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":3}}}`)
	}))
	defer server.Close()
	req := testRequest()
	req.ResponseSchema = []byte(`{"type":"object","properties":{"issues":{"type":"array"}},"required":["issues"],"additionalProperties":false}`)
	resp, err := testProvider(t, server).Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "正文" || resp.ProviderRequestID != "provider-123" || len(resp.Raw) == 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if !resp.Usage.Reported || resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 4 || resp.Usage.CachedInputTokens != 3 {
		t.Fatalf("incorrect usage: %+v", resp.Usage)
	}
}

func TestCompatibleMissingUsageIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"正文"}}]}`)
	}))
	defer server.Close()
	resp, err := testProvider(t, server).Generate(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.Reported {
		t.Fatal("missing usage must not be reported as zero cost")
	}
}

func TestCompatibleHTTPFailuresDoNotRetry(t *testing.T) {
	for _, tc := range []struct {
		status      int
		disposition string
	}{
		{http.StatusTooManyRequests, Retryable},
		{http.StatusUnauthorized, Permanent},
		{http.StatusBadRequest, Permanent},
		{http.StatusInternalServerError, NeedsReconciliation},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":{"message":"test-secret should never appear in errors"}}`)
			}))
			defer server.Close()
			resp, err := testProvider(t, server).Generate(context.Background(), testRequest())
			if err == nil || FailureDisposition(err, true) != tc.disposition {
				t.Fatalf("got %v (%s), expected %s", err, FailureDisposition(err, true), tc.disposition)
			}
			if strings.Contains(err.Error(), "test-secret") {
				t.Fatal("credential leaked through remote error message")
			}
			if len(resp.Raw) == 0 {
				t.Fatal("failed response was not retained")
			}
			if calls.Load() != 1 {
				t.Fatalf("retried request: %d calls", calls.Load())
			}
		})
	}
}

func TestCompatibleRejectsMalformedResponses(t *testing.T) {
	for _, body := range []string{`not json`, `{}`, `{"choices":[]}`, `{"choices":[{"message":{"content":""}}]}`, `{"choices":[{"message":{"content":"正文"}}],"usage":{"prompt_tokens":-1,"completion_tokens":1}}`} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, body) }))
			defer server.Close()
			resp, err := testProvider(t, server).Generate(context.Background(), testRequest())
			if err == nil || FailureDisposition(err, true) != NeedsReconciliation {
				t.Fatalf("malformed successful response must need reconciliation: %v", err)
			}
			if string(resp.Raw) != body {
				t.Fatal("raw response lost")
			}
			if calls.Load() != 1 {
				t.Fatal("unknown result was retried")
			}
		})
	}
}

func TestCompatibleInterruptedResponseDoesNotRetry(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	defer server.Close()
	_, err := testProvider(t, server).Generate(context.Background(), testRequest())
	if err == nil || FailureDisposition(err, true) != NeedsReconciliation {
		t.Fatalf("request accepted then disconnected: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("unknown result was retried")
	}
}

func TestCompatibleTimeoutDoesNotRetry(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer server.Close()
	provider := testProvider(t, server)
	provider.client.Timeout = 25 * time.Millisecond
	_, err := provider.Generate(context.Background(), testRequest())
	if err == nil || FailureDisposition(err, true) != NeedsReconciliation {
		t.Fatalf("timeout after dispatch: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("unknown result was retried")
	}
}

func TestCompatibleConnectionFailureCanRetry(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	provider, err := NewCompatible(Config{BaseURL: "http://" + address + "/v1", APIKey: "test-secret", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), testRequest())
	if err == nil || FailureDisposition(err, false) != Retryable {
		t.Fatalf("connection refused before dispatch: %v", err)
	}
}

func TestCompatibleResponseSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 200)) }))
	defer server.Close()
	provider, err := NewCompatible(Config{BaseURL: server.URL, APIKey: "test-secret", MaxResponseBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := provider.Generate(context.Background(), testRequest())
	if err == nil || FailureDisposition(err, true) != NeedsReconciliation {
		t.Fatalf("oversized result: %v", err)
	}
	if len(resp.Raw) != 64 {
		t.Fatalf("raw response must be bounded, got %d", len(resp.Raw))
	}
}

func TestCompatiblePreflightAndRedirect(t *testing.T) {
	for _, config := range []Config{
		{BaseURL: "https://example.test/v1"},
		{BaseURL: "file:///tmp/provider", APIKey: "secret"},
		{BaseURL: "https://user:secret@example.test/v1", APIKey: "secret"},
		{BaseURL: "https://example.test/v1?key=secret", APIKey: "secret"},
	} {
		if _, err := NewCompatible(config); err == nil {
			t.Fatalf("invalid config was accepted: %+v", config)
		}
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := testProvider(t, server).Generate(context.Background(), testRequest())
	if err == nil || calls.Load() != 1 {
		t.Fatalf("redirect must not replay model POST: %v, calls=%d", err, calls.Load())
	}
}

func TestFailureDisposition(t *testing.T) {
	if got := FailureDisposition(errors.New("local connection failure"), false); got != Retryable {
		t.Fatal(got)
	}
	if got := FailureDisposition(errors.New("unknown execution"), true); got != NeedsReconciliation {
		t.Fatal(got)
	}
	if got := FailureDisposition(nil, false); got != "" {
		t.Fatal(got)
	}
}

func TestFakeRequiresExplicitHandler(t *testing.T) {
	if _, err := (&Fake{}).Generate(context.Background(), testRequest()); err == nil {
		t.Fatal("unconfigured fake must not pretend to generate")
	}
}
