package chat

import (
	"context"
	"iter"
)

// Model is the minimal synchronous chat capability. Implementations must
// validate req before provider I/O and return a provider-neutral Response.
//
// Streaming, default configuration, and provider identity are independent
// concerns and deliberately are not methods of Model.
type Model interface {
	Call(ctx context.Context, req *Request) (*Response, error)
}

// ModelFunc adapts an ordinary function to Model, following the same pattern
// as net/http.HandlerFunc.
type ModelFunc func(ctx context.Context, req *Request) (*Response, error)

// Call invokes f.
func (f ModelFunc) Call(ctx context.Context, req *Request) (*Response, error) {
	return f(ctx, req)
}

// Streamer is the optional streaming chat capability. It is independent of
// Model so an implementation is not forced to provide a synthetic synchronous
// Call path, and a call-only implementation is not forced to fake streaming.
type Streamer interface {
	Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error]
}

// StreamerFunc adapts an ordinary function to Streamer.
type StreamerFunc func(ctx context.Context, req *Request) iter.Seq2[*Response, error]

// Stream invokes f.
func (f StreamerFunc) Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return f(ctx, req)
}
