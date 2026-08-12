package mutation

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
