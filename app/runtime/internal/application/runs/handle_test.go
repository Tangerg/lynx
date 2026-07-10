package runs

import (
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
		committed, err := h.commitInterrupt(func() error {
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
	committed, err := h.commitInterrupt(func() error {
		called = true
		return nil
	})
	if err != nil || committed || called {
		t.Fatalf("post-cancel commit = committed:%v called:%v err:%v", committed, called, err)
	}
}
