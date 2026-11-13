package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestResponsesRequestAndOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Errorf("wrong endpoint: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Openai-Actor-Authorization") != "unit-test-actor-header" {
			t.Error("missing configured header")
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing API key")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if string(body["store"]) != "false" {
			t.Error("responses must explicitly disable storage")
		}
		for _, field := range []string{"tools", "tool_choice", "web_search", "messages", "max_tokens", "response_format", "temperature"} {
			if _, exists := body[field]; exists {
				t.Errorf("unexpected Responses field %s", field)
			}
		}
		if string(body["max_output_tokens"]) != "300" || string(body["model"]) != `"test-model"` {
			t.Errorf("incorrect request: %s", body)
		}
		var input []Message
		if err := json.Unmarshal(body["input"], &input); err != nil || len(input) != 1 || input[0].Content != "请审稿" {
			t.Errorf("invalid input: %s", body["input"])
		}
		var text struct {
			Format struct {
				Type, Name string
				Strict     bool
				Schema     json.RawMessage
			}
		}
		if err := json.Unmarshal(body["text"], &text); err != nil || text.Format.Type != "json_schema" || text.Format.Name != "review_issues" || !text.Format.Strict || len(text.Format.Schema) == 0 {
			t.Errorf("incorrect text.format: %s", body["text"])
		}
		fmt.Fprint(w, `{"id":"resp_123","status":"completed","output":[{"type":"reasoning","summary":[]},{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"第一段。"},{"type":"output_text","text":"第二段。"}]}],"usage":{"input_tokens":30,"output_tokens":8,"input_tokens_details":{"cached_tokens":5}}}`)
	}))
	defer server.Close()
	provider, err := NewCompatible(Config{BaseURL: server.URL + "/v1", APIKey: "test-secret", WireAPI: "responses", Headers: map[string]string{"x-openai-actor-authorization": "unit-test-actor-header"}})
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest()
	request.ResponseSchema = []byte(`{"type":"object","properties":{"issues":{"type":"array"}},"required":["issues"],"additionalProperties":false}`)
	response, err := provider.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "第一段。第二段。" || response.ProviderRequestID != "resp_123" || len(response.Raw) == 0 {
		t.Fatalf("incorrect response: %+v", response)
	}
	if !response.Usage.Reported || response.Usage.InputTokens != 30 || response.Usage.OutputTokens != 8 || response.Usage.CachedInputTokens != 5 {
		t.Fatalf("incorrect usage: %+v", response.Usage)
	}
}

func TestResponsesIncompleteAndMissingUsage(t *testing.T) {
	for _, test := range []struct {
		status, output string
		failed         bool
	}{
		{"completed", `[{"type":"message","content":[{"type":"output_text","text":"正文"}]}]`, false},
		{"incomplete", `[{"type":"message","content":[{"type":"output_text","text":"未完成的正文"}]}]`, true},
		{"failed", `[]`, true},
		{"completed", `[{"type":"message","content":[{"type":"refusal","refusal":"拒绝"}]}]`, true},
	} {
		t.Run(test.status+test.output, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				fmt.Fprintf(w, `{"id":"resp_test","status":%q,"output":%s}`, test.status, test.output)
			}))
			defer server.Close()
			provider, err := NewCompatible(Config{BaseURL: server.URL, APIKey: "test-secret", WireAPI: "responses"})
			if err != nil {
				t.Fatal(err)
			}
			response, err := provider.Generate(context.Background(), testRequest())
			if (err != nil) != test.failed {
				t.Fatalf("unexpected failure: %v", err)
			}
			if response.Usage.Reported {
				t.Fatal("missing usage was treated as reported")
			}
			if len(response.Raw) == 0 || calls.Load() != 1 {
				t.Fatal("missing raw response or replayed request")
			}
			if test.status == "incomplete" && response.Text != "未完成的正文" {
				t.Fatal("partial output was discarded")
			}
		})
	}
}

func TestResponsesIncompleteRetainsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"partial","status":"incomplete","output":[],"usage":{"input_tokens":20,"output_tokens":30}}`)
	}))
	defer server.Close()
	provider, err := NewCompatible(Config{BaseURL: server.URL, APIKey: "test-secret", WireAPI: "responses"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), testRequest())
	if err == nil || FailureDisposition(err, true) != NeedsReconciliation {
		t.Fatalf("incomplete result must fail: %v", err)
	}
	if !response.Usage.Reported || response.Usage.InputTokens != 20 || response.Usage.OutputTokens != 30 {
		t.Fatal("partial response cost was discarded")
	}
}

func TestResponsesRedirectDoesNotReplay(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	provider, err := NewCompatible(Config{BaseURL: server.URL, APIKey: "test-secret", WireAPI: "responses"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), testRequest())
	if err == nil || calls.Load() != 1 {
		t.Fatalf("redirect replayed: err=%v calls=%d", err, calls.Load())
	}
}

func TestConfiguredHeadersAreCopiedAndValidated(t *testing.T) {
	for _, headers := range []map[string]string{
		{"Authorization": "other-secret"}, {"Content-Type": "text/plain"}, {"Host": "other.test"}, {"X-Test": "value\r\nAuthorization: secret"}, {"Bad Header": "secret"},
	} {
		if _, err := NewCompatible(Config{BaseURL: "https://example.test/v1", APIKey: "test-secret", WireAPI: "responses", Headers: headers}); err == nil || strings.Contains(err.Error(), "other-secret") {
			t.Fatal("invalid header accepted or leaked")
		}
	}
	if _, err := NewCompatible(Config{BaseURL: "https://example.test/v1", APIKey: "test-secret", WireAPI: "unknown"}); err == nil {
		t.Fatal("unknown wire API accepted")
	}
	headers := map[string]string{"X-Client": "original"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Client") != "original" {
			t.Error("config header map was not copied")
		}
		fmt.Fprint(w, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"正文"}]}]}`)
	}))
	defer server.Close()
	provider, err := NewCompatible(Config{BaseURL: server.URL, APIKey: "test-secret", WireAPI: "responses", Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	headers["X-Client"] = "mutated"
	if _, err := provider.Generate(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
}
