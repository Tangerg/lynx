package teardown

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestStepSharesActiveAttempt(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	step := Retryable(func(context.Context) error {
		calls.Add(1)
		<-release
		return nil
	})

	first := step.Begin(t.Context())
	second := step.Begin(t.Context())
	if complete, _ := first.Result(); complete {
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

func TestStepRetriesCompletedFailure(t *testing.T) {
	want := errors.New("close failed")
	var calls atomic.Int32
	step := Retryable(func(context.Context) error {
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

func TestStepJoinersObserveSameFailure(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	want := errors.New("first close failed")
	var calls atomic.Int32
	step := Retryable(func(context.Context) error {
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
		t.Fatalf("joining attempt = %v, want same failure", err)
	}
	if err := step.Begin(t.Context()).Wait(t.Context()); err != nil {
		t.Fatalf("explicit retry = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("action calls after retry = %d, want 2", got)
	}
}

func TestTerminalStepSettlesWithDiagnostic(t *testing.T) {
	want := errors.New("terminal close diagnostic")
	var calls atomic.Int32
	step := Terminal(func(context.Context) error {
		calls.Add(1)
		return want
	})

	settled, err := step.Shutdown(t.Context())
	if !settled || !errors.Is(err, want) {
		t.Fatalf("first shutdown = (settled=%v, err=%v), want terminal diagnostic", settled, err)
	}
	settled, err = step.Shutdown(t.Context())
	if !settled || !errors.Is(err, want) {
		t.Fatalf("second shutdown = (settled=%v, err=%v), want immutable settlement", settled, err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("terminal action calls = %d, want 1", got)
	}
}

func TestAttemptReturnsCompletedResultAfterCallerCancellation(t *testing.T) {
	want := errors.New("close failed")
	attempt := completedAttempt(want)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := attempt.Wait(ctx); !errors.Is(err, want) {
		t.Fatalf("wait = %v, want completed result", err)
	}
}
