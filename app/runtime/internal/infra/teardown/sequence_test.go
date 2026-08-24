package teardown

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestSequenceClosesEveryTerminalStepInReverseOrder(t *testing.T) {
	firstErr := errors.New("first diagnostic")
	lastErr := errors.New("last diagnostic")
	var calls []int
	sequence := NewSequence([]*Step{
		Terminal(func(context.Context) error {
			calls = append(calls, 1)
			return firstErr
		}),
		nil,
		Terminal(func(context.Context) error {
			calls = append(calls, 3)
			return lastErr
		}),
	})

	settled, err := sequence.Shutdown(t.Context())
	if !settled || !errors.Is(err, firstErr) || !errors.Is(err, lastErr) {
		t.Fatalf("Shutdown = (settled=%v, err=%v), want both terminal diagnostics", settled, err)
	}
	if !slices.Equal(calls, []int{3, 1}) {
		t.Fatalf("close order = %v, want [3 1]", calls)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	settled, err = sequence.Shutdown(canceled)
	if !settled || !errors.Is(err, firstErr) || !errors.Is(err, lastErr) {
		t.Fatalf("completed Shutdown with canceled caller = (settled=%v, err=%v), want immutable result", settled, err)
	}
	if !slices.Equal(calls, []int{3, 1}) {
		t.Fatalf("repeated Shutdown replayed steps: %v", calls)
	}
}

func TestSequenceContinuesAfterCallerTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	dependencyClosed := make(chan struct{})
	var calls atomic.Int32
	sequence := NewSequence([]*Step{
		Terminal(func(context.Context) error {
			close(dependencyClosed)
			return nil
		}),
		Terminal(func(context.Context) error {
			calls.Add(1)
			close(started)
			<-release
			return nil
		}),
	})

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	settled, err := sequence.Shutdown(ctx)
	if settled || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown = (settled=%v, err=%v), want in-flight timeout", settled, err)
	}
	<-started
	type shutdownResult struct {
		settled bool
		err     error
	}
	joined := make(chan shutdownResult, 1)
	go func() {
		settled, err := sequence.Shutdown(t.Context())
		joined <- shutdownResult{settled: settled, err: err}
	}()
	close(release)
	select {
	case <-dependencyClosed:
	case <-time.After(time.Second):
		t.Fatal("Sequence abandoned its dependency graph after caller timeout")
	}

	result := <-joined
	if !result.settled || result.err != nil {
		t.Fatalf("joined Shutdown = (settled=%v, err=%v), want completed graph", result.settled, result.err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("blocking closer calls = %d, want 1", got)
	}
}
