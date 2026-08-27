package chat

import (
	"context"
	"iter"
)

// Model is the minimal synchronous chat capability. Implementations must
// validate request before provider I/O, honor context cancellation, and return a
// provider-neutral Response. Cancellation errors must retain
// context.Canceled or context.DeadlineExceeded for errors.Is.
//
// Streaming, default configuration, and provider identity are independent
// concerns and deliberately are not methods of Model.
type Model interface {
	Call(ctx context.Context, request *Request) (*Response, error)
}

type ModelFunc func(ctx context.Context, request *Request) (*Response, error)

func (m ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return m(ctx, request)
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
	Stream(ctx context.Context, request *Request) iter.Seq2[*Response, error]
}

type StreamerFunc func(ctx context.Context, request *Request) iter.Seq2[*Response, error]

func (s StreamerFunc) Stream(ctx context.Context, request *Request) iter.Seq2[*Response, error] {
	return s(ctx, request)
}
