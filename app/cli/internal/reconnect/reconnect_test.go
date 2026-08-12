package reconnect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestReconnectRetriesOnlyTransientErrorsWithinBudget(t *testing.T) {
	policy := Policy{Attempts: 3, Base: 10 * time.Millisecond, Maximum: 25 * time.Millisecond}
	for failure, want := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond} {
		got, ok := policy.Next(failure+1, agent.ErrDisconnected)
		if !ok || got != want {
			t.Fatalf("failure %d = %s, %v; want %s", failure+1, got, ok, want)
		}
	}
	if _, ok := policy.Next(4, agent.ErrDisconnected); ok {
		t.Fatal("retry budget was exceeded")
	}
	if _, ok := policy.Next(1, agent.ErrEventConflict); ok {
		t.Fatal("identity conflict was treated as transient")
	}
	if _, ok := policy.Next(1, agent.ErrReplayUnavailable); ok {
		t.Fatal("unavailable replay was treated as a retryable disconnect")
	}
}

func TestWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v", err)
	}
}

func TestBackoffBoundsAnOperationOwnedRetrySchedule(t *testing.T) {
	backoff := Backoff{Base: 100 * time.Millisecond, Maximum: 5 * time.Second}
	for failure, want := range []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		3200 * time.Millisecond,
		5 * time.Second,
		5 * time.Second,
	} {
		if got := backoff.Delay(failure + 1); got != want {
			t.Fatalf("failure %d delay = %s, want %s", failure+1, got, want)
		}
	}
}
