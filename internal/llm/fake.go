package llm

import (
	"context"
	"errors"
)

// Fake requires an explicit scenario. It does not estimate model quality or
// silently return a made-up success when the test harness is not configured.
type Fake struct {
	Handler func(context.Context, Request) (Response, error)
}

func (f *Fake) Generate(ctx context.Context, req Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if f == nil || f.Handler == nil {
		return Response{}, errors.New("fake provider requires an explicit handler")
	}
	return f.Handler(ctx, req)
}
