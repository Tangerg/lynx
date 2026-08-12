package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
