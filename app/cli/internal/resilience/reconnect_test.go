package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestReconnectRetriesOnlyTransientErrorsWithinBudget(t *testing.T) {
	policy := Reconnect{Attempts: 3, Base: 10 * time.Millisecond, Maximum: 25 * time.Millisecond}
	for failure, want := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond} {
		got, ok := policy.Next(failure+1, client.ErrDisconnected)
		if !ok || got != want {
			t.Fatalf("failure %d = %s, %v; want %s", failure+1, got, ok, want)
		}
	}
	if _, ok := policy.Next(4, client.ErrDisconnected); ok {
		t.Fatal("retry budget was exceeded")
	}
	if _, ok := policy.Next(1, client.ErrEventConflict); ok {
		t.Fatal("identity conflict was treated as transient")
	}
	if _, ok := policy.Next(1, client.ErrEventGap); !ok {
		t.Fatal("replayable gap was not treated as transient")
	}
}

func TestWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v", err)
	}
}
