package speech

import (
	"context"
	"iter"
)

// Model is the synchronous provider-neutral speech generation SPI.
type Model interface {
	Call(context.Context, *Request) (*Response, error)
}

// ModelFunc adapts a function to [Model].
type ModelFunc func(context.Context, *Request) (*Response, error)

func (f ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return f(ctx, request)
}

// Streamer is the optional streaming capability. It is independent from
// [Model], so callers only require streaming when they consume it.
type Streamer interface {
	Stream(context.Context, *Request) iter.Seq2[*Response, error]
}

// StreamFunc adapts a function to [Streamer].
type StreamFunc func(context.Context, *Request) iter.Seq2[*Response, error]

func (f StreamFunc) Stream(ctx context.Context, request *Request) iter.Seq2[*Response, error] {
	return f(ctx, request)
}
