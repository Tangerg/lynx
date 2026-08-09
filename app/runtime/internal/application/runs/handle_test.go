package runs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestHandleCancelLinearizesAfterInterruptCommit: requestCancel must not return
// while an interrupt commit is in flight (they share the handle lock), and a
// post-cancel commit is refused — the invariant that stops a cancel from
// deleting an interrupt the pump is about to publish.
func TestHandleCancelLinearizesAfterInterruptCommit(t *testing.T) {
	commitStarted := make(chan struct{})
	releaseCommit := make(chan struct{})
	requested := make(chan struct{})
	h := &handle{}

	commitDone := make(chan struct{})
	go func() {
		defer close(commitDone)
		committed, err := h.commitInterrupt(t.Context(), func(context.Context) error {
			close(commitStarted)
			<-releaseCommit
			return nil
		})
		if err != nil || !committed {
			t.Errorf("commitInterrupt = committed:%v err:%v, want committed", committed, err)
		}
	}()
	<-commitStarted

	cancelDone := make(chan struct{})
	go func() {
		committed, err := h.requestCancel(t.Context(), "user canceled", func(context.Context) error {
			close(requested)
			return nil
		})
		if err != nil || !committed {
			t.Errorf("requestCancel = (%v, %v), want committed", committed, err)
		}
		close(cancelDone)
	}()
	select {
	case <-cancelDone:
		t.Fatal("cancel crossed an in-flight interrupt commit")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseCommit)
	select {
	case <-commitDone:
	case <-time.After(time.Second):
		t.Fatal("interrupt commit did not finish")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("cancel did not continue after interrupt commit")
	}
	select {
	case <-requested:
	case <-time.After(time.Second):
		t.Fatal("executor cancellation was not requested")
	}
	if got := h.CancelReason(); got != "user canceled" {
		t.Fatalf("cancel reason = %q", got)
	}

	called := false
	committed, err := h.commitInterrupt(t.Context(), func(context.Context) error {
		called = true
		return nil
	})
	if err != nil || committed || called {
		t.Fatalf("post-cancel commit = committed:%v called:%v err:%v", committed, called, err)
	}
}

func TestHandleCancelInterruptsBlockedCommit(t *testing.T) {
	commitStarted := make(chan struct{})
	h := &handle{}
	commitResult := make(chan error, 1)
	go func() {
		committed, err := h.commitInterrupt(t.Context(), func(ctx context.Context) error {
			close(commitStarted)
			<-ctx.Done()
			return ctx.Err()
		})
		if committed {
			commitResult <- errors.New("blocked interrupt unexpectedly committed")
			return
		}
		commitResult <- err
	}()
	<-commitStarted

	cancelDone := make(chan struct{})
	go func() {
		committed, err := h.requestCancel(t.Context(), "user canceled", acceptRootCancel)
		if err != nil || committed {
			t.Errorf("requestCancel = (%v, %v), want no committed interrupt", committed, err)
		}
		close(cancelDone)
	}()

	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt and join the blocked commit")
	}
	if err := <-commitResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("commit error = %v, want context.Canceled", err)
	}
}

func TestHandleRetainsCommittedInterruptOutcomeAfterCommitReturns(t *testing.T) {
	h := &handle{}
	committed, err := h.commitInterrupt(t.Context(), func(context.Context) error {
		return nil
	})
	if err != nil || !committed {
		t.Fatalf("commitInterrupt = (%v, %v), want committed", committed, err)
	}

	interruptCommitted, err := h.requestCancel(t.Context(), "user canceled", acceptRootCancel)
	if err != nil || !interruptCommitted {
		t.Fatalf("requestCancel = (%v, %v), want retained committed outcome", interruptCommitted, err)
	}
}

func TestHandleCancelWaitIsContextBounded(t *testing.T) {
	commitStarted := make(chan struct{})
	releaseCommit := make(chan struct{})
	h := &handle{}
	commitDone := make(chan struct{})
	go func() {
		defer close(commitDone)
		_, _ = h.commitInterrupt(t.Context(), func(context.Context) error {
			close(commitStarted)
			<-releaseCommit
			return nil
		})
	}()
	<-commitStarted

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.requestCancel(ctx, "user canceled", acceptRootCancel); !errors.Is(err, context.Canceled) {
		t.Fatalf("requestCancel error = %v, want context canceled", err)
	}
	close(releaseCommit)
	<-commitDone
}

func TestHandleCleanupContextDetachesFinishedOwner(t *testing.T) {
	type contextKey struct{}
	owner, cancelOwner := context.WithCancel(context.WithValue(t.Context(), contextKey{}, "trace"))
	cancelOwner()
	h := &handle{owner: owner}

	cleanup, cancelCleanup := h.cleanupContext(context.Background())
	defer cancelCleanup()
	if cleanup.Err() != nil {
		t.Fatalf("cleanup inherited finished owner cancellation: %v", cleanup.Err())
	}
	if got := cleanup.Value(contextKey{}); got != "trace" {
		t.Fatalf("cleanup context value = %v, want trace", got)
	}
	if _, ok := cleanup.Deadline(); !ok {
		t.Fatal("cleanup context is not bounded")
	}
}

func TestHandleWaitReturnsCompletedOutcomeAfterCallerCancellation(t *testing.T) {
	want := errors.New("run cleanup failed")
	done := make(chan struct{})
	close(done)
	h := &handle{done: done, completionErr: want}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := h.wait(ctx); !errors.Is(err, want) {
		t.Fatalf("wait() = %v, want completed outcome", err)
	}
}

func TestHandleCancellationArbiterAllowsOnlyOneTreeOwner(t *testing.T) {
	plan := runningChildCancellationPlan()

	t.Run("child wins", func(t *testing.T) {
		canceled := false
		h := &handle{cancel: func() { canceled = true }}
		attempt, err := h.beginChildCancellation(plan, "stop child")
		if err != nil {
			t.Fatalf("begin child cancellation: %v", err)
		}
		if _, err := h.requestCancel(t.Context(), "stop root", acceptRootCancel); !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("root cancellation error = %v, want ErrSessionBusy", err)
		}
		if canceled || h.cancelRequested || h.cancelReason != "" {
			t.Fatalf(
				"losing root cancellation mutated owner state: canceled=%t requested=%t reason=%q",
				canceled,
				h.cancelRequested,
				h.cancelReason,
			)
		}
		h.abortChildCancellation(attempt, errors.New("test complete"))
	})

	t.Run("root wins", func(t *testing.T) {
		h := &handle{}
		if _, err := h.requestCancel(t.Context(), "stop root", acceptRootCancel); err != nil {
			t.Fatalf("request root cancellation: %v", err)
		}
		if _, err := h.beginChildCancellation(plan, "stop child"); !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("child cancellation error = %v, want ErrSessionBusy", err)
		} else if !strings.Contains(err.Error(), plan.root.run.ID) {
			t.Fatalf("child cancellation error = %q, want root identity", err)
		}
	})
}

func acceptRootCancel(context.Context) error { return nil }
