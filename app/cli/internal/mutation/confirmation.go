// Package mutation owns acknowledgement semantics for idempotent commands.
package mutation

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
)

// AcknowledgementUncertain reports whether a mutation may have committed even
// though its acknowledgement was not observed. Callers must retry the same
// command identity; a fresh identity could execute the user's intent twice.
func AcknowledgementUncertain(err error) bool {
	return errors.Is(err, agent.ErrDisconnected) ||
		errors.Is(err, agent.ErrCommandInProgress) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// Confirm retries one idempotent mutation until its acknowledgement is
// observed, a definitive refusal arrives, or the owning context ends. The
// attempt closure must capture and reuse the same mutation identity.
func Confirm[T any](ctx context.Context, backoff retry.Backoff, attempt func(context.Context) (T, error)) (T, error) {
	for failures := 0; ; {
		result, err := attempt(ctx)
		if err == nil || !AcknowledgementUncertain(err) {
			return result, err
		}
		failures++
		if err := retry.Wait(ctx, backoff.Delay(failures)); err != nil {
			var zero T
			return zero, err
		}
	}
}
