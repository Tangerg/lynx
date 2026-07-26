package shutdown

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestBeginStartsOnceAndSharesAttempt(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	step := New(func(context.Context) error {
		calls.Add(1)
		<-release
		return nil
	})

	first := step.Begin(t.Context())
	second := step.Begin(t.Context())
	if _, complete := first.Result(); complete {
		t.Fatal("attempt completed before action was released")
	}
	close(release)
	if err := first.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := second.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("action calls = %d, want 1", got)
	}
}

func TestFailedAttemptIsRetryable(t *testing.T) {
	want := errors.New("close failed")
	var calls atomic.Int32
	step := New(func(context.Context) error {
		if calls.Add(1) == 1 {
			return want
		}
		return nil
	})

	if err := step.Begin(t.Context()).Wait(t.Context()); !errors.Is(err, want) {
		t.Fatalf("first attempt = %v, want failure", err)
	}
	if err := step.Begin(t.Context()).Wait(t.Context()); err != nil {
		t.Fatalf("retry = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("action calls = %d, want 2", got)
	}
}

func TestCallerJoiningFailedAttemptObservesSameGeneration(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	want := errors.New("first close failed")
	var calls atomic.Int32
	step := New(func(context.Context) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			return want
		}
		return nil
	})

	first := step.Begin(t.Context())
	<-entered
	second := step.Begin(t.Context())
	close(release)

	if err := first.Wait(t.Context()); !errors.Is(err, want) {
		t.Fatalf("first attempt = %v, want failure", err)
	}
	if err := second.Wait(t.Context()); !errors.Is(err, want) {
		t.Fatalf("joining caller attempt = %v, want same failure", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("action calls = %d, want 1", got)
	}
	if err := step.Begin(t.Context()).Wait(t.Context()); err != nil {
		t.Fatalf("explicit retry = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("action calls after explicit retry = %d, want 2", got)
	}
}

func TestAttemptWaitReturnsCompletedOutcomeAfterCallerCancellation(t *testing.T) {
	want := errors.New("close failed")
	attempt := completedAttempt(want)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := attempt.Wait(ctx); !errors.Is(err, want) {
		t.Fatalf("Wait() = %v, want completed outcome", err)
	}
}
