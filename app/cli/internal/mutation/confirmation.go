// Package mutation owns acknowledgement semantics for idempotent commands.
package mutation

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
)

// ErrReplayGuaranteeUnavailable fences a command whose stable identity can no
// longer be proven replayable by the runtime that originally owned it.
var ErrReplayGuaranteeUnavailable = errors.New("mutation replay guarantee is unavailable")

// Outcome is the authoritative settlement state shared by every durable CLI
// mutation. The zero value is invalid so an uninitialized result cannot be
// mistaken for a deliberately preserved unknown outcome.
type Outcome string

const (
	Unknown   Outcome = "unknown"
	Rejected  Outcome = "rejected"
	Confirmed Outcome = "confirmed"
)

func (outcome Outcome) Valid() bool {
	switch outcome {
	case Unknown, Rejected, Confirmed:
		return true
	default:
		return false
	}
}

func (outcome Outcome) String() string { return string(outcome) }

// Admission runs immediately before each real mutation attempt. Durable
// callers use it to enforce the runtime replay guarantee at the actual I/O
// boundary rather than only when a recovery workflow begins.
type Admission func() error

// ReplayUntil admits attempts strictly before one conservative replay
// deadline. A nil clock uses the process wall clock.
func ReplayUntil(until time.Time, now func() time.Time) Admission {
	return func() error {
		current := time.Now().UTC()
		if now != nil {
			current = now().UTC()
		}
		if !current.Before(until) {
			return ErrReplayGuaranteeUnavailable
		}
		return nil
	}
}

// AcknowledgementUncertain reports whether a mutation may have committed even
// though its acknowledgement was not observed. Callers must retry the same
// command identity; a fresh identity could execute the user's intent twice.
func AcknowledgementUncertain(err error) bool {
	return errors.Is(err, agent.ErrDisconnected) ||
		errors.Is(err, agent.ErrCommandInProgress) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// OutcomeUnknown includes failures which cannot be retried against the current
// runtime but still do not prove that an earlier attempt was refused. A store
// mismatch is fenced before current-store admission; it says nothing about the
// same command's outcome in the store that originally owned it.
func OutcomeUnknown(err error) bool {
	return AcknowledgementUncertain(err) ||
		errors.Is(err, agent.ErrCommandStoreMismatch) ||
		errors.Is(err, ErrReplayGuaranteeUnavailable)
}

// Confirm retries one idempotent mutation until its acknowledgement is
// observed, a definitive refusal arrives, or the owning context ends. The
// attempt closure must capture and reuse the same mutation identity.
func Confirm[T any](ctx context.Context, backoff retry.Backoff, attempt func(context.Context) (T, error)) (T, error) {
	return ConfirmAdmitted(ctx, backoff, nil, attempt)
}

// ConfirmAdmitted has the same acknowledgement semantics as Confirm and also
// requires admission immediately before every attempt, including the first.
// An admission failure is an unknown outcome: an earlier call may have
// committed even though its replay guarantee has since expired.
func ConfirmAdmitted[T any](
	ctx context.Context,
	backoff retry.Backoff,
	admit Admission,
	attempt func(context.Context) (T, error),
) (T, error) {
	for failures := 0; ; {
		if admit != nil {
			if err := admit(); err != nil {
				var zero T
				return zero, err
			}
		}
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
