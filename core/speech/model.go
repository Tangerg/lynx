package speech

import (
	"context"
	"iter"
)

// Model is the synchronous provider-neutral speech generation SPI. Call
// implementations validate requests before I/O, reject explicit options they
// cannot represent, preserve context error identity, and return responses that
// pass Validate.
type Model interface {
	Call(context.Context, *Request) (*Response, error)
}

// ModelFunc adapts a function to [Model].
type ModelFunc func(context.Context, *Request) (*Response, error)

func (f ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return f(ctx, request)
}

// Streamer is the optional streaming capability. Every yielded response obeys
// the [Model] response contract. It is independent from [Model], so callers only
// require streaming when they consume it.
type Streamer interface {
	Stream(context.Context, *Request) iter.Seq2[*Response, error]
}

// StreamerFunc adapts a function to [Streamer].
type StreamerFunc func(context.Context, *Request) iter.Seq2[*Response, error]

func (f StreamerFunc) Stream(ctx context.Context, request *Request) iter.Seq2[*Response, error] {
	return f(ctx, request)
}
