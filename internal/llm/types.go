package llm

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model           string
	Messages        []Message
	MaxOutputTokens int
	Temperature     *float64
	TopP            *float64
	ResponseSchema  []byte
}

type Usage struct {
	InputTokens       int64 `json:"inputTokens"`
	OutputTokens      int64 `json:"outputTokens"`
	CachedInputTokens int64 `json:"cachedInputTokens"`
	Reported          bool  `json:"reported"`
}

type Response struct {
	Text              string
	Raw               []byte
	ProviderRequestID string
	Usage             Usage
}

type Provider interface {
	Generate(context.Context, Request) (Response, error)
}

type Delta struct {
	Text string `json:"text"`
}

// StreamingProvider emits actual provider text increments. Returning an error
// from emit stops consumption; the external generation may already be billed.
type StreamingProvider interface {
	Stream(context.Context, Request, func(Delta) error) (Response, error)
}
