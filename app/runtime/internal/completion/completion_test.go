package completion

import (
	"context"
	"errors"
	"testing"
)

func TestWaitPrefersObservableCompletionOverCancellation(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	close(done)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := Wait(ctx, done); err != nil {
		t.Fatalf("Wait() = %v, want completed", err)
	}
}

func TestWaitReportsCancellationWhileCompletionIsPending(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := Wait(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() = %v, want context.Canceled", err)
	}
}
