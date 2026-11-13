package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type Config struct {
	AllowNoAuth      bool
	BaseURL          string
	APIKey           string
	Model            string
	WireAPI          string
	Headers          map[string]string
	Timeout          time.Duration
	MaxResponseBytes int64
}

type Compatible struct {
	endpoint         string
	apiKey           string
	model            string
	wireAPI          string
	headers          map[string]string
	maxResponseBytes int64
	client           *http.Client
}

func NewCompatible(config Config) (*Compatible, error) {
	base, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("model base URL must be an HTTP(S) URL without credentials, query, or fragment")
	}
	key := strings.TrimSpace(config.APIKey)
	if (key == "" && !config.AllowNoAuth) || strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("model API key is missing or invalid")
	}
	if config.Timeout < 0 || config.MaxResponseBytes < 0 || config.MaxResponseBytes > 64<<20 {
		return nil, errors.New("invalid model timeout or response size limit")
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = 8 << 20
	}
	wireAPI := config.WireAPI
	if wireAPI == "" {
		wireAPI = "chat"
	}
	if wireAPI != "chat" && wireAPI != "responses" {
		return nil, errors.New("model wire API must be chat or responses")
	}
	headers := make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		if !validHeaderName(name) || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("invalid configured model header")
		}
		switch http.CanonicalHeaderKey(name) {
		case "Authorization", "Host", "Content-Type", "Content-Length", "Accept", "Transfer-Encoding", "Connection":
			return nil, errors.New("configured model headers cannot override transport or authentication headers")
		}
		headers[name] = value
	}
	suffix := "/chat/completions"
	if wireAPI == "responses" {
		suffix = "/responses"
	}
	base.Path = strings.TrimRight(base.Path, "/") + suffix
	base.RawPath = ""
	return &Compatible{
		endpoint: base.String(), apiKey: key, model: strings.TrimSpace(config.Model), wireAPI: wireAPI, headers: headers, maxResponseBytes: config.MaxResponseBytes,
		client: &http.Client{
			Timeout: config.Timeout,
			// Redirects could replay a billable POST or forward credentials. A base
			// URL must point to the intended endpoint directly.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

type compatibleRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	MaxTokens      int             `json:"max_tokens"`
	Temperature    *float64        `json:"temperature,omitempty"`
	TopP           *float64        `json:"top_p,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Store          bool            `json:"store"`
}

type responsesRequest struct {
	Model           string         `json:"model"`
	Input           []Message      `json:"input"`
	MaxOutputTokens int            `json:"max_output_tokens"`
	Store           bool           `json:"store"`
	Temperature     *float64       `json:"temperature,omitempty"`
	TopP            *float64       `json:"top_p,omitempty"`
	Text            *responsesText `json:"text,omitempty"`
}

type responsesText struct {
	Format responsesFormat `json:"format"`
}

type responsesFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type responseFormat struct {
	Type       string         `json:"type"`
	JSONSchema responseSchema `json:"json_schema"`
}

type responseSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

func (p *Compatible) Generate(ctx context.Context, req Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, &Failure{Disposition: Permanent, Kind: "cancelled_before_dispatch"}
	}
	encoded, err := p.encodeRequest(req, false)
	if err != nil {
		return Response{}, err
	}
	var sent atomic.Bool
	trace := &httptrace.ClientTrace{
		WroteHeaders: func() { sent.Store(true) },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				sent.Store(true)
			}
		},
	}
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, p.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Response{}, &Failure{Disposition: Permanent, Kind: "invalid_request"}
	}
	if p.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	for name, value := range p.headers {
		request.Header.Set(name, value)
	}
	// No retries here. The caller records the attempt before entering Generate.
	response, err := p.client.Do(request)
	if err != nil {
		disposition := Retryable
		if sent.Load() {
			disposition = NeedsReconciliation
		}
		return Response{}, &Failure{Disposition: disposition, Kind: "transport"}
	}
	defer response.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, p.maxResponseBytes+1))
	oversized := int64(len(raw)) > p.maxResponseBytes
	if oversized {
		raw = raw[:p.maxResponseBytes]
	}
	result := Response{Raw: raw, ProviderRequestID: response.Header.Get("X-Request-ID")}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		disposition := Permanent
		if response.StatusCode == http.StatusTooManyRequests {
			disposition = Retryable
		} else if response.StatusCode >= 500 || response.StatusCode == http.StatusRequestTimeout {
			// Generic compatible providers do not guarantee that a failed HTTP
			// response means the generation was never executed or charged.
			disposition = NeedsReconciliation
		}
		return result, &Failure{Disposition: disposition, Kind: "http_status", StatusCode: response.StatusCode}
	}
	if oversized {
		return result, &Failure{Disposition: NeedsReconciliation, Kind: "response_too_large"}
	}
	if readErr != nil {
		return result, &Failure{Disposition: NeedsReconciliation, Kind: "response_interrupted"}
	}
	if p.wireAPI == "responses" {
		return decodeResponses(result)
	}
	return decodeChat(result)
}

func decodeChat(result Response) (Response, error) {
	var decoded struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     *int64 `json:"prompt_tokens"`
			CompletionTokens *int64 `json:"completion_tokens"`
			PromptDetails    *struct {
				CachedTokens *int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(result.Raw, &decoded); err != nil || len(decoded.Choices) == 0 {
		return result, &Failure{Disposition: NeedsReconciliation, Kind: "invalid_response"}
	}
	if decoded.ID != "" {
		result.ProviderRequestID = decoded.ID
	}
	if err := json.Unmarshal(decoded.Choices[0].Message.Content, &result.Text); err != nil || strings.TrimSpace(result.Text) == "" {
		return result, &Failure{Disposition: NeedsReconciliation, Kind: "missing_text_output"}
	}
	if usage := decoded.Usage; usage != nil {
		if usage.PromptTokens != nil && usage.CompletionTokens != nil {
			result.Usage = Usage{InputTokens: *usage.PromptTokens, OutputTokens: *usage.CompletionTokens, Reported: true}
			if usage.PromptDetails != nil && usage.PromptDetails.CachedTokens != nil {
				result.Usage.CachedInputTokens = *usage.PromptDetails.CachedTokens
			}
			if result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || result.Usage.CachedInputTokens < 0 || result.Usage.CachedInputTokens > result.Usage.InputTokens {
				result.Usage.Reported = false
				return result, &Failure{Disposition: NeedsReconciliation, Kind: "invalid_usage"}
			}
		}
	}
	return result, nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			continue
		}
		return false
	}
	return true
}

func decodeResponses(result Response) (Response, error) {
	var decoded struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage *struct {
			InputTokens  *int64 `json:"input_tokens"`
			OutputTokens *int64 `json:"output_tokens"`
			InputDetails *struct {
				CachedTokens *int64 `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(result.Raw, &decoded); err != nil {
		return result, &Failure{Disposition: NeedsReconciliation, Kind: "invalid_response"}
	}
	if decoded.ID != "" {
		result.ProviderRequestID = decoded.ID
	}
	if usage := decoded.Usage; usage != nil && usage.InputTokens != nil && usage.OutputTokens != nil {
		result.Usage = Usage{InputTokens: *usage.InputTokens, OutputTokens: *usage.OutputTokens, Reported: true}
		if usage.InputDetails != nil && usage.InputDetails.CachedTokens != nil {
			result.Usage.CachedInputTokens = *usage.InputDetails.CachedTokens
		}
		if result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || result.Usage.CachedInputTokens < 0 || result.Usage.CachedInputTokens > result.Usage.InputTokens {
			result.Usage.Reported = false
			return result, &Failure{Disposition: NeedsReconciliation, Kind: "invalid_usage"}
		}
	}
	var text strings.Builder
	messageIncomplete := false
	for _, item := range decoded.Output {
		if item.Type != "message" || (item.Role != "" && item.Role != "assistant") {
			continue
		}
		if item.Status != "" && item.Status != "completed" {
			messageIncomplete = true
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				text.WriteString(content.Text)
			}
		}
	}
	result.Text = text.String()
	if decoded.Status != "completed" || messageIncomplete {
		return result, &Failure{Disposition: NeedsReconciliation, Kind: "incomplete_response"}
	}
	if strings.TrimSpace(result.Text) == "" {
		return result, &Failure{Disposition: NeedsReconciliation, Kind: "missing_text_output"}
	}
	return result, nil
}

func (p *Compatible) encodeRequest(req Request, stream bool) ([]byte, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" || req.MaxOutputTokens <= 0 || len(req.Messages) == 0 {
		return nil, &Failure{Disposition: Permanent, Kind: "invalid_request"}
	}
	for _, message := range req.Messages {
		if (message.Role != "system" && message.Role != "user" && message.Role != "assistant") || strings.TrimSpace(message.Content) == "" {
			return nil, &Failure{Disposition: Permanent, Kind: "invalid_message"}
		}
	}
	if req.Temperature != nil && (math.IsNaN(*req.Temperature) || math.IsInf(*req.Temperature, 0) || *req.Temperature < 0 || *req.Temperature > 2) {
		return nil, &Failure{Disposition: Permanent, Kind: "invalid_temperature"}
	}
	if req.TopP != nil && (math.IsNaN(*req.TopP) || math.IsInf(*req.TopP, 0) || *req.TopP < 0 || *req.TopP > 1) {
		return nil, &Failure{Disposition: Permanent, Kind: "invalid_top_p"}
	}
	payload := compatibleRequest{Model: model, Messages: req.Messages, MaxTokens: req.MaxOutputTokens, Temperature: req.Temperature, TopP: req.TopP}
	if len(req.ResponseSchema) != 0 {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(req.ResponseSchema, &object); err != nil || object == nil {
			return nil, &Failure{Disposition: Permanent, Kind: "invalid_response_schema"}
		}
		payload.ResponseFormat = &responseFormat{Type: "json_schema", JSONSchema: responseSchema{Name: "novel_response", Strict: true, Schema: req.ResponseSchema}}
	}
	var body any = payload
	if p.wireAPI == "responses" {
		responsesPayload := responsesRequest{Model: model, Input: req.Messages, MaxOutputTokens: req.MaxOutputTokens, Temperature: req.Temperature, TopP: req.TopP, Store: false}
		if len(req.ResponseSchema) != 0 {
			responsesPayload.Text = &responsesText{Format: responsesFormat{Type: "json_schema", Name: "review_issues", Strict: true, Schema: req.ResponseSchema}}
		}
		body = responsesPayload
	}
	if stream {
		raw, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, &Failure{Disposition: Permanent, Kind: "request_encoding"}
		}
		var object map[string]any
		if json.Unmarshal(raw, &object) != nil {
			return nil, &Failure{Disposition: Permanent, Kind: "request_encoding"}
		}
		object["stream"] = true
		if p.wireAPI == "chat" {
			object["stream_options"] = map[string]bool{"include_usage": true}
		}
		body = object
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, &Failure{Disposition: Permanent, Kind: "request_encoding"}
	}
	return encoded, nil
}
