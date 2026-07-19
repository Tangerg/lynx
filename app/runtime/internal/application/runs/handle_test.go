package runs

import (
	"context"
	"errors"
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
	canceled := make(chan struct{})
	h := &handle{cancel: func() { close(canceled) }}

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
		h.requestCancel("user canceled")
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
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("run context was not canceled")
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
		h.requestCancel("user canceled")
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
