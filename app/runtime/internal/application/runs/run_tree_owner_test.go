package runs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestRunTreeOwnerCancelLinearizesAfterInterruptCommit proves requestCancel
// does not return while an interrupt commit is in flight and that a post-cancel
// commit is refused. Both operations share the Run-tree owner's lock.
func TestRunTreeOwnerCancelLinearizesAfterInterruptCommit(t *testing.T) {
	commitStarted := make(chan struct{})
	releaseCommit := make(chan struct{})
	requested := make(chan struct{})
	treeOwner := &runTreeOwner{}

	commitDone := make(chan struct{})
	go func() {
		defer close(commitDone)
		committed, err := treeOwner.commitInterrupt(t.Context(), func(context.Context) error {
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
		committed, err := treeOwner.requestCancel(t.Context(), "user canceled", func(context.Context) error {
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
	if got := treeOwner.CancelReason(); got != "user canceled" {
		t.Fatalf("cancel reason = %q", got)
	}

	called := false
	committed, err := treeOwner.commitInterrupt(t.Context(), func(context.Context) error {
		called = true
		return nil
	})
	if err != nil || committed || called {
		t.Fatalf("post-cancel commit = committed:%v called:%v err:%v", committed, called, err)
	}
}

func TestRunTreeOwnerCancelInterruptsBlockedCommit(t *testing.T) {
	commitStarted := make(chan struct{})
	treeOwner := &runTreeOwner{}
	commitResult := make(chan error, 1)
	go func() {
		committed, err := treeOwner.commitInterrupt(t.Context(), func(ctx context.Context) error {
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
		committed, err := treeOwner.requestCancel(t.Context(), "user canceled", acceptRootCancel)
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

func TestRunTreeOwnerCancelWaitsForSegmentActivation(t *testing.T) {
	activationStarted := make(chan struct{})
	releaseActivation := make(chan struct{})
	cancelRequested := make(chan struct{})
	treeOwner := &runTreeOwner{
		activation: segmentActivation{done: make(chan struct{})},
	}

	activationDone := make(chan error, 1)
	go func() {
		_, err := treeOwner.beginExecution(t.Context(), func(context.Context) error {
			close(activationStarted)
			<-releaseActivation
			return nil
		})
		activationDone <- err
	}()
	<-activationStarted

	cancelDone := make(chan error, 1)
	go func() {
		_, err := treeOwner.requestCancel(t.Context(), "stop", func(context.Context) error {
			close(cancelRequested)
			return nil
		})
		cancelDone <- err
	}()
	select {
	case <-cancelRequested:
		t.Fatal("cancel crossed an in-flight segment activation")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseActivation)
	if err := <-activationDone; err != nil {
		t.Fatalf("activate segment: %v", err)
	}
	if err := <-cancelDone; err != nil {
		t.Fatalf("cancel after activation: %v", err)
	}
	select {
	case <-cancelRequested:
	case <-time.After(time.Second):
		t.Fatal("cancel did not continue after segment activation")
	}
}

func TestRunTreeOwnerCancelBeforeActivationSuppressesExecutorBegin(t *testing.T) {
	treeOwner := &runTreeOwner{
		activation: segmentActivation{done: make(chan struct{})},
	}
	if _, err := treeOwner.requestCancel(t.Context(), "stop", acceptRootCancel); err != nil {
		t.Fatalf("request cancel: %v", err)
	}

	beginCalled := false
	canceled, err := treeOwner.beginExecution(t.Context(), func(context.Context) error {
		beginCalled = true
		return nil
	})
	if err != nil || !canceled || beginCalled {
		t.Fatalf(
			"begin after cancel = canceled:%t called:%t err:%v, want true/false/nil",
			canceled,
			beginCalled,
			err,
		)
	}
}

func TestRunTreeOwnerCancelClassifiesActivationFailureAsFinished(t *testing.T) {
	treeOwner := &runTreeOwner{
		activation: segmentActivation{done: make(chan struct{})},
	}
	if _, err := treeOwner.beginExecution(t.Context(), func(context.Context) error {
		return errors.New("activation failed")
	}); err == nil {
		t.Fatal("segment activation unexpectedly succeeded")
	}
	if _, err := treeOwner.requestCancel(t.Context(), "stop", acceptRootCancel); !errors.Is(err, ErrRunFinished) {
		t.Fatalf("cancel after activation failure = %v, want ErrRunFinished", err)
	}
}

func TestRunTreeOwnerRetainsCommittedInterruptOutcomeAfterCommitReturns(t *testing.T) {
	treeOwner := &runTreeOwner{}
	committed, err := treeOwner.commitInterrupt(t.Context(), func(context.Context) error {
		return nil
	})
	if err != nil || !committed {
		t.Fatalf("commitInterrupt = (%v, %v), want committed", committed, err)
	}

	interruptCommitted, err := treeOwner.requestCancel(t.Context(), "user canceled", acceptRootCancel)
	if err != nil || !interruptCommitted {
		t.Fatalf("requestCancel = (%v, %v), want retained committed outcome", interruptCommitted, err)
	}
}

func TestRunTreeOwnerCancelWaitIsContextBounded(t *testing.T) {
	commitStarted := make(chan struct{})
	releaseCommit := make(chan struct{})
	treeOwner := &runTreeOwner{}
	commitDone := make(chan struct{})
	go func() {
		defer close(commitDone)
		_, _ = treeOwner.commitInterrupt(t.Context(), func(context.Context) error {
			close(commitStarted)
			<-releaseCommit
			return nil
		})
	}()
	<-commitStarted

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := treeOwner.requestCancel(ctx, "user canceled", acceptRootCancel); !errors.Is(err, context.Canceled) {
		t.Fatalf("requestCancel error = %v, want context canceled", err)
	}
	close(releaseCommit)
	<-commitDone
}

func TestRunTreeOwnerCleanupContextDetachesFinishedTask(t *testing.T) {
	type contextKey struct{}
	owner, cancelOwner := context.WithCancel(context.WithValue(t.Context(), contextKey{}, "trace"))
	cancelOwner()
	treeOwner := &runTreeOwner{taskContext: owner}

	cleanup, cancelCleanup := treeOwner.cleanupContext(context.Background())
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

func TestRunTreeOwnerWaitReturnsCompletedOutcomeAfterCallerCancellation(t *testing.T) {
	want := errors.New("run cleanup failed")
	done := make(chan struct{})
	close(done)
	treeOwner := &runTreeOwner{done: done, completionErr: want}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := treeOwner.wait(ctx); !errors.Is(err, want) {
		t.Fatalf("wait() = %v, want completed outcome", err)
	}
}

func TestRunTreeOwnerCancellationArbiterAllowsOnlyOneTreeOwner(t *testing.T) {
	plan := runningChildCancellationPlan()

	t.Run("child wins", func(t *testing.T) {
		canceled := false
		treeOwner := &runTreeOwner{cancel: func() { canceled = true }}
		attempt, err := treeOwner.beginChildCancellation(plan, "stop child")
		if err != nil {
			t.Fatalf("begin child cancellation: %v", err)
		}
		if _, err := treeOwner.requestCancel(t.Context(), "stop root", acceptRootCancel); !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("root cancellation error = %v, want ErrSessionBusy", err)
		}
		if canceled || treeOwner.cancelRequested || treeOwner.cancelReason != "" {
			t.Fatalf(
				"losing root cancellation mutated owner state: canceled=%t requested=%t reason=%q",
				canceled,
				treeOwner.cancelRequested,
				treeOwner.cancelReason,
			)
		}
		treeOwner.abortChildCancellation(attempt, errors.New("test complete"))
	})

	t.Run("root wins", func(t *testing.T) {
		treeOwner := &runTreeOwner{}
		if _, err := treeOwner.requestCancel(t.Context(), "stop root", acceptRootCancel); err != nil {
			t.Fatalf("request root cancellation: %v", err)
		}
		if _, err := treeOwner.beginChildCancellation(plan, "stop child"); !errors.Is(err, ErrSessionBusy) {
			t.Fatalf("child cancellation error = %v, want ErrSessionBusy", err)
		} else if !strings.Contains(err.Error(), plan.root.run.ID()) {
			t.Fatalf("child cancellation error = %q, want root identity", err)
		}
	})
}

func acceptRootCancel(context.Context) error { return nil }
