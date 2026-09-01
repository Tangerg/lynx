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
	// Call performs one complete model exchange. It must reject an invalid
	// request before provider I/O, must not retain or mutate request, and
	// transfers ownership of the returned response to the caller. Context
	// cancellation remains identifiable through errors.Is.
	Call(ctx context.Context, request *Request) (*Response, error)
}

// ModelFunc adapts a function to Model without introducing a second call API.
type ModelFunc func(ctx context.Context, request *Request) (*Response, error)

func (m ModelFunc) Call(ctx context.Context, request *Request) (*Response, error) {
	return m(ctx, request)
}

// Streamer is the optional streaming chat capability. It is independent of
// Model so an implementation is not forced to provide a synthetic synchronous
// Call path, and a call-only implementation is not forced to fake streaming.
//
// Every successful yield is a valid ResponseDelta. Usage, when present, is a
// cumulative snapshot rather than a per-chunk increment. On failure the
// sequence yields (nil, err) once and terminates. Context errors retain their
// errors.Is identity. When the caller stops iteration, implementations must
// synchronously release provider resources without yielding a cancellation
// error or leaving a detached goroutine behind. [ResponseAccumulator] defines
// the provider-neutral aggregation semantics.
type Streamer interface {
	// Stream starts provider work lazily when the sequence is iterated. Each
	// yielded response is an independently owned delta accepted by
	// ResponseAccumulator. Stopping iteration releases provider resources before
	// the iterator returns; a terminal error is yielded at most once.
	Stream(ctx context.Context, request *Request) iter.Seq2[*ResponseDelta, error]
}

// StreamerFunc adapts a function to Streamer without coupling it to Model.
type StreamerFunc func(ctx context.Context, request *Request) iter.Seq2[*ResponseDelta, error]

func (s StreamerFunc) Stream(ctx context.Context, request *Request) iter.Seq2[*ResponseDelta, error] {
	return s(ctx, request)
}
