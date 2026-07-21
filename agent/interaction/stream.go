package interaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// StreamCall drives streamer for one request and returns the single response
// the synchronous managed-interaction port consumes. It forwards every stream
// delta to onDelta (nil to ignore) and merges the deltas into one response via
// [chat.ResponseAccumulator].
//
// Cancellation, idle, and retry policy stay with the caller through ctx and the
// supplied streamer; StreamCall owns only accumulation and delta forwarding. It
// reports an error when the stream fails, yields a nil delta, cannot be
// accumulated, or ends without producing any delta. onDelta observes a delta
// only after it has been accumulated, so a delta it sees is always valid.
func StreamCall(ctx context.Context, streamer chat.Streamer, req *chat.Request, onDelta func(*chat.Response)) (*chat.Response, error) {
	if streamer == nil {
		return nil, errors.New("interaction: nil streamer")
	}
	var accumulator chat.ResponseAccumulator
	seen := false
	for delta, err := range streamer.Stream(ctx, req) {
		if err != nil {
			return nil, err
		}
		if delta == nil {
			return nil, errors.New("interaction: chat stream yielded a nil delta")
		}
		if err := accumulator.Add(delta); err != nil {
			return nil, fmt.Errorf("interaction: accumulate chat stream: %w", err)
		}
		seen = true
		if onDelta != nil {
			onDelta(delta)
		}
	}
	if !seen {
		return nil, errors.New("interaction: chat stream ended without a delta")
	}
	return accumulator.Response(), nil
}
