package chat

import (
	"context"
	"iter"
)

// Model is the minimal synchronous chat capability. Implementations must
// validate req before provider I/O, honor context cancellation, and return a
// provider-neutral Response. Cancellation errors must retain
// context.Canceled or context.DeadlineExceeded for errors.Is.
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
//
// Every successful yield is a valid response delta. Usage, when present, is a
// cumulative snapshot rather than a per-chunk increment. On failure the
// sequence yields (nil, err) once and terminates. Context errors retain their
// errors.Is identity. When the caller stops iteration, implementations must
// synchronously release provider resources without yielding a cancellation
// error or leaving a detached goroutine behind. [ResponseAccumulator] defines
// the provider-neutral aggregation semantics.
type Streamer interface {
	Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error]
}

// StreamerFunc adapts an ordinary function to Streamer.
type StreamerFunc func(ctx context.Context, req *Request) iter.Seq2[*Response, error]

// Stream invokes f.
func (f StreamerFunc) Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return f(ctx, req)
}
