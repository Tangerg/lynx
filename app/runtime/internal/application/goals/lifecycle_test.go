package goals

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionCommandLocksDoNotCoupleUnrelatedSessions(t *testing.T) {
	mutations := NewSessionMutations()
	releaseFirst := mutations.acquire("ses_1")

	unrelatedAcquired := make(chan struct{})
	releaseUnrelated := make(chan struct{})
	unrelatedDone := make(chan struct{})
	go func() {
		defer close(unrelatedDone)
		release := mutations.acquire("ses_2")
		close(unrelatedAcquired)
		<-releaseUnrelated
		release()
	}()
	select {
	case <-unrelatedAcquired:
	case <-time.After(time.Second):
		t.Fatal("unrelated session command was blocked")
	}

	sameAcquired := make(chan struct{})
	releaseSame := make(chan struct{})
	sameDone := make(chan struct{})
	go func() {
		defer close(sameDone)
		release := mutations.acquire("ses_1")
		close(sameAcquired)
		<-releaseSame
		release()
	}()
	select {
	case <-sameAcquired:
		t.Fatal("same-session command crossed the ownership lock")
	default:
	}

	releaseFirst()
	select {
	case <-sameAcquired:
	case <-time.After(time.Second):
		t.Fatal("same-session command did not continue after release")
	}
	close(releaseSame)
	close(releaseUnrelated)
	<-sameDone
	<-unrelatedDone
}

func TestSessionMutationQuiescesEveryGoalBeforeJoining(t *testing.T) {
	firstErr := errors.New("first goal cleanup failed")
	first := completedLoopHandle(firstErr)
	second := completedLoopHandle(nil)
	var firstCanceled atomic.Bool
	var secondCanceled atomic.Bool
	first.cancel = func() { firstCanceled.Store(true) }
	second.cancel = func() { secondCanceled.Store(true) }

	mutations := NewSessionMutations()
	mutations.running["ses_1"] = first
	mutations.running["ses_2"] = second

	err := mutations.WithSessionMutation(
		t.Context(),
		[]string{"ses_1", "ses_2"},
		func(context.Context) error { return nil },
	)
	if !errors.Is(err, firstErr) {
		t.Fatalf("WithSessionMutation error = %v, want first cleanup failure", err)
	}
	if !firstCanceled.Load() || !secondCanceled.Load() {
		t.Fatalf("quiesced goals = first:%v second:%v, want both", firstCanceled.Load(), secondCanceled.Load())
	}
	if mutations.driverLease("ses_1") != "" || mutations.driverLease("ses_2") != "" {
		t.Fatal("successful session mutation retained a goal driver")
	}
}

func TestQuiesceRetainsJoinIdentityUntilDriverExits(t *testing.T) {
	mutations := NewSessionMutations()
	var canceled atomic.Bool
	handle := &loopHandle{
		leaseID:      "lease_1",
		cancel:       func() { canceled.Store(true) },
		done:         make(chan struct{}),
		stopResolved: make(chan stopResolution, 1),
	}
	mutations.running["ses_1"] = handle
	driver := &Driver{mutations: mutations}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := driver.quiesceDrive(ctx, "ses_1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("quiesceDrive error = %v, want canceled wait", err)
	}
	if !canceled.Load() {
		t.Fatal("quiesce did not cancel the driver")
	}
	if got := mutations.driverLease("ses_1"); got != "lease_1" {
		t.Fatalf("driver lease after timed-out join = %q, want retained ownership", got)
	}

	mutations.forget("ses_1", handle)
	close(handle.done)
	if got := mutations.driverLease("ses_1"); got != "" {
		t.Fatalf("driver lease after owner exit = %q, want released", got)
	}
}

func TestOwnedRunStartsCancellationOnce(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	run := newOwnedRun(t.Context(), func(context.Context) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil
	})

	run.stop()
	run.stop()
	<-started
	if got := calls.Load(); got != 1 {
		t.Fatalf("cancel calls = %d, want one", got)
	}
	close(release)
	if err := run.wait(t.Context()); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("cancel calls after wait = %d, want one", got)
	}
}

func completedLoopHandle(err error) *loopHandle {
	done := make(chan struct{})
	close(done)
	return &loopHandle{
		done:         done,
		err:          err,
		cancel:       func() {},
		stopResolved: make(chan stopResolution, 1),
	}
}
