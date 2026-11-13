package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
)

// Stream consumes provider SSE events directly. There is exactly one HTTP
// request, including for servers that ignore stream and return ordinary JSON.
func (p *Compatible) Stream(ctx context.Context, req Request, emit func(Delta) error) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, &Failure{Disposition: Permanent, Kind: "cancelled_before_dispatch"}
	}
	encoded, err := p.encodeRequest(req, true)
	if err != nil {
		return Response{}, err
	}
	var sent atomic.Bool
	trace := &httptrace.ClientTrace{WroteHeaders: func() { sent.Store(true) }, WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			sent.Store(true)
		}
	}}
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, p.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Response{}, &Failure{Disposition: Permanent, Kind: "invalid_request"}
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	for name, value := range p.headers {
		request.Header.Set(name, value)
	}
	response, err := p.client.Do(request)
	if err != nil {
		disposition := Retryable
		if sent.Load() {
			disposition = NeedsReconciliation
		}
		return Response{}, &Failure{Disposition: disposition, Kind: "transport"}
	}
	defer response.Body.Close()
	result := Response{ProviderRequestID: response.Header.Get("X-Request-ID")}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Raw, _ = io.ReadAll(io.LimitReader(response.Body, p.maxResponseBytes))
		disposition := Permanent
		if response.StatusCode == http.StatusTooManyRequests {
			disposition = Retryable
		} else if response.StatusCode >= 500 || response.StatusCode == http.StatusRequestTimeout {
			disposition = NeedsReconciliation
		}
		return result, &Failure{Disposition: disposition, Kind: "http_status", StatusCode: response.StatusCode}
	}
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if contentType != "text/event-stream" {
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, p.maxResponseBytes+1))
		result.Raw = raw
		if int64(len(raw)) > p.maxResponseBytes {
			result.Raw = raw[:p.maxResponseBytes]
			return result, &Failure{Disposition: NeedsReconciliation, Kind: "response_too_large"}
		}
		if readErr != nil {
			return result, &Failure{Disposition: NeedsReconciliation, Kind: "response_interrupted"}
		}
		if p.wireAPI == "responses" {
			result, err = decodeResponses(result)
		} else {
			result, err = decodeChat(result)
		}
		if err == nil && emit != nil && result.Text != "" {
			if emit(Delta{Text: result.Text}) != nil {
				return result, &Failure{Disposition: NeedsReconciliation, Kind: "stream_consumer"}
			}
		}
		return result, err
	}
	var raw bytes.Buffer
	reader := bufio.NewReader(io.TeeReader(io.LimitReader(response.Body, p.maxResponseBytes+1), &raw))
	var data []string
	var text strings.Builder
	chatFinished := false
	fail := func(kind string) (Response, error) {
		result.Text = text.String()
		result.Raw = append([]byte(nil), raw.Bytes()...)
		if int64(len(result.Raw)) > p.maxResponseBytes {
			result.Raw = result.Raw[:p.maxResponseBytes]
		}
		return result, &Failure{Disposition: NeedsReconciliation, Kind: kind}
	}
	add := func(delta string) error {
		if delta == "" {
			return nil
		}
		text.WriteString(delta)
		if emit != nil {
			return emit(Delta{Text: delta})
		}
		return nil
	}
	process := func(payload string) (bool, error) {
		if payload == "[DONE]" {
			if p.wireAPI != "chat" || !chatFinished || strings.TrimSpace(text.String()) == "" {
				return false, &Failure{Disposition: NeedsReconciliation, Kind: "incomplete_stream"}
			}
			return true, nil
		}
		if p.wireAPI == "responses" {
			var event struct {
				Type     string          `json:"type"`
				Delta    string          `json:"delta"`
				Response json.RawMessage `json:"response"`
			}
			if json.Unmarshal([]byte(payload), &event) != nil {
				return false, &Failure{Disposition: NeedsReconciliation, Kind: "invalid_stream_event"}
			}
			switch event.Type {
			case "response.output_text.delta":
				if err := add(event.Delta); err != nil {
					return false, &Failure{Disposition: NeedsReconciliation, Kind: "stream_consumer"}
				}
			case "response.completed", "response.failed", "response.incomplete":
				decoded, decodeErr := decodeResponses(Response{Raw: event.Response, ProviderRequestID: result.ProviderRequestID})
				result.ProviderRequestID = decoded.ProviderRequestID
				result.Usage = decoded.Usage
				if decodeErr != nil || event.Type != "response.completed" {
					return false, &Failure{Disposition: NeedsReconciliation, Kind: "incomplete_stream"}
				}
				// Actual final provider output may include a final unstreamed suffix, but
				// never manufacture incremental characters or replace divergent output.
				if !strings.HasPrefix(decoded.Text, text.String()) {
					return false, &Failure{Disposition: NeedsReconciliation, Kind: "stream_output_mismatch"}
				}
				if err := add(strings.TrimPrefix(decoded.Text, text.String())); err != nil {
					return false, &Failure{Disposition: NeedsReconciliation, Kind: "stream_consumer"}
				}
				return true, nil
			case "error":
				return false, &Failure{Disposition: NeedsReconciliation, Kind: "stream_provider_error"}
			}
			return false, nil
		}
		var chunk struct {
			ID      string          `json:"id"`
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Index int `json:"index"`
				Delta struct {
					Content string `json:"content"`
					Refusal string `json:"refusal"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				Input   *int64 `json:"prompt_tokens"`
				Output  *int64 `json:"completion_tokens"`
				Details *struct {
					Cached *int64 `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			return false, &Failure{Disposition: NeedsReconciliation, Kind: "invalid_stream_event"}
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return false, &Failure{Disposition: NeedsReconciliation, Kind: "stream_provider_error"}
		}
		if chunk.ID != "" {
			result.ProviderRequestID = chunk.ID
		}
		if u := chunk.Usage; u != nil && u.Input != nil && u.Output != nil {
			result.Usage = Usage{InputTokens: *u.Input, OutputTokens: *u.Output, Reported: true}
			if u.Details != nil && u.Details.Cached != nil {
				result.Usage.CachedInputTokens = *u.Details.Cached
			}
			if result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || result.Usage.CachedInputTokens < 0 || result.Usage.CachedInputTokens > result.Usage.InputTokens {
				result.Usage.Reported = false
				return false, &Failure{Disposition: NeedsReconciliation, Kind: "invalid_usage"}
			}
		}
		for _, choice := range chunk.Choices {
			if choice.Index != 0 {
				continue
			}
			if choice.Delta.Refusal != "" {
				return false, &Failure{Disposition: NeedsReconciliation, Kind: "refused_output"}
			}
			if chatFinished && choice.Delta.Content != "" {
				return false, &Failure{Disposition: NeedsReconciliation, Kind: "stream_output_after_finish"}
			}
			if err := add(choice.Delta.Content); err != nil {
				return false, &Failure{Disposition: NeedsReconciliation, Kind: "stream_consumer"}
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				if *choice.FinishReason != "stop" {
					return false, &Failure{Disposition: NeedsReconciliation, Kind: "incomplete_stream"}
				}
				chatFinished = true
			}
		}
		return false, nil
	}
	for {
		line, readErr := reader.ReadString('\n')
		if int64(raw.Len()) > p.maxResponseBytes {
			return fail("response_too_large")
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" && len(data) > 0 {
			done, eventErr := process(strings.Join(data, "\n"))
			data = nil
			if eventErr != nil {
				var failure *Failure
				if f, ok := eventErr.(*Failure); ok {
					failure = f
				}
				if failure != nil {
					return fail(failure.Kind)
				}
				return fail("invalid_stream_event")
			}
			if done {
				result.Text = text.String()
				result.Raw = append([]byte(nil), raw.Bytes()...)
				return result, nil
			}
		} else if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			data = append(data, value)
		}
		if readErr != nil {
			return fail("stream_interrupted")
		}
	}
}
