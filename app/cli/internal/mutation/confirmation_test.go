package mutation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
)

func TestAcknowledgementUncertainIncludesMutationTimeouts(t *testing.T) {
	for _, err := range []error{
		agent.ErrDisconnected,
		agent.ErrCommandInProgress,
		context.Canceled,
		context.DeadlineExceeded,
		fmt.Errorf("adapter timeout: %w", context.DeadlineExceeded),
	} {
		if !AcknowledgementUncertain(err) {
			t.Fatalf("AcknowledgementUncertain(%v) = false", err)
		}
	}
	for _, err := range []error{nil, agent.ErrSessionHasActiveRun, agent.ErrCommandConflict} {
		if AcknowledgementUncertain(err) {
			t.Fatalf("AcknowledgementUncertain(%v) = true", err)
		}
	}
	if AcknowledgementUncertain(agent.ErrCommandStoreMismatch) {
		t.Fatal("a runtime-store mismatch must not be retried against the same store")
	}
	if !OutcomeUnknown(agent.ErrCommandStoreMismatch) {
		t.Fatal("a runtime-store mismatch discarded an unknown prior-store outcome")
	}
}

func TestOutcomeHasOneSharedStableIdentity(t *testing.T) {
	tests := []struct {
		outcome Outcome
		want    string
	}{
		{outcome: Unknown, want: "unknown"},
		{outcome: Rejected, want: "rejected"},
		{outcome: Confirmed, want: "confirmed"},
	}
	for _, test := range tests {
		if !test.outcome.Valid() || test.outcome.String() != test.want {
			t.Fatalf("outcome = %q, valid = %t; want %q", test.outcome, test.outcome.Valid(), test.want)
		}
	}
	if Outcome("").Valid() {
		t.Fatal("zero Outcome is valid")
	}
}

func TestConfirmStopsAtARuntimeStoreMismatch(t *testing.T) {
	attempts := 0
	_, err := Confirm(t.Context(), retry.Backoff{}, func(context.Context) (struct{}, error) {
		attempts++
		return struct{}{}, agent.ErrCommandStoreMismatch
	})
	if !errors.Is(err, agent.ErrCommandStoreMismatch) || attempts != 1 {
		t.Fatalf("confirmation error = %v after %d attempts", err, attempts)
	}
}

func TestConfirmRetriesAnUncertainMutationWithTheSameOwner(t *testing.T) {
	attempts := 0
	result, err := Confirm(t.Context(), retry.Backoff{}, func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", context.DeadlineExceeded
		}
		return "acknowledged", nil
	})
	if err != nil || result != "acknowledged" || attempts != 3 {
		t.Fatalf("confirmation = %q, %v after %d attempts", result, err, attempts)
	}
}

func TestConfirmStopsAtOwnerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	attempts := 0
	_, err := Confirm(ctx, retry.Backoff{}, func(context.Context) (struct{}, error) {
		attempts++
		cancel()
		return struct{}{}, context.DeadlineExceeded
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("confirmation error = %v after %d attempts", err, attempts)
	}
}

func TestConfirmAdmittedFencesEveryRuntimeAttempt(t *testing.T) {
	replayable := true
	attempts := 0
	_, err := ConfirmAdmitted(
		t.Context(), retry.Backoff{},
		func() error {
			if !replayable {
				return ErrReplayGuaranteeUnavailable
			}
			return nil
		},
		func(context.Context) (struct{}, error) {
			attempts++
			replayable = false
			return struct{}{}, agent.ErrDisconnected
		},
	)
	if !errors.Is(err, ErrReplayGuaranteeUnavailable) || !OutcomeUnknown(err) {
		t.Fatalf("confirmation error = %v, unknown = %t", err, OutcomeUnknown(err))
	}
	if attempts != 1 {
		t.Fatalf("expired command reached runtime %d times", attempts)
	}
}

func TestReplayAdmissionExpiresAtItsDeadline(t *testing.T) {
	deadline := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		err := ReplayUntil(deadline, func() time.Time { return deadline.Add(offset) })()
		if offset < 0 && err != nil {
			t.Fatalf("admission before deadline = %v", err)
		}
		if offset >= 0 && !errors.Is(err, ErrReplayGuaranteeUnavailable) {
			t.Fatalf("admission at offset %s = %v", offset, err)
		}
	}
}
