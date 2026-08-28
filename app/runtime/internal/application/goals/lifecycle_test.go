package goals

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

type alreadyTerminalRuns struct{}

func (alreadyTerminalRuns) WaitSessionStartable(context.Context, string) error { return nil }
func (alreadyTerminalRuns) AdmitSelection(modelref.Selection) error            { return nil }
func (alreadyTerminalRuns) Start(context.Context, runs.StartCommand) (runs.StartResult, error) {
	return runs.StartResult{}, nil
}
func (alreadyTerminalRuns) Cancel(context.Context, runs.CancelCommand) (runs.CancelResult, error) {
	return runs.CancelResult{}, runs.ErrRunFinished
}

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
	first := completedGoalDrive(firstErr)
	second := completedGoalDrive(nil)
	var firstCanceled atomic.Bool
	var secondCanceled atomic.Bool
	first.cancel = func() { firstCanceled.Store(true) }
	second.cancel = func() { secondCanceled.Store(true) }

	mutations := NewSessionMutations()
	mutations.drives["ses_1"] = first
	mutations.drives["ses_2"] = second

	err := mutations.WithSessionMutation(
		t.Context(),
		[]string{"ses_1", "ses_2"},
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
	)
	if !errors.Is(err, firstErr) {
		t.Fatalf("WithSessionMutation error = %v, want first cleanup failure", err)
	}
	if !firstCanceled.Load() || !secondCanceled.Load() {
		t.Fatalf("quiesced goals = first:%v second:%v, want both", firstCanceled.Load(), secondCanceled.Load())
	}
	if mutations.activeDrive("ses_1") != nil || mutations.activeDrive("ses_2") != nil {
		t.Fatal("successful session mutation retained a goal driver")
	}
}

func TestQuiesceRetainsJoinIdentityUntilDriverExits(t *testing.T) {
	mutations := NewSessionMutations()
	var canceled atomic.Bool
	drive := &goalDrive{
		incarnationID: "lease_1",
		cancel:        func() { canceled.Store(true) },
		done:          make(chan struct{}),
	}
	mutations.drives["ses_1"] = drive
	driver := &Driver{mutations: mutations}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := driver.quiesceDrive(ctx, "ses_1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("quiesceDrive error = %v, want canceled wait", err)
	}
	if !canceled.Load() {
		t.Fatal("quiesce did not cancel the driver")
	}
	if got := mutations.activeDrive("ses_1"); got != drive {
		t.Fatalf("drive after timed-out join = %p, want retained drive %p", got, drive)
	}

	mutations.forget("ses_1", drive)
	close(drive.done)
	if got := mutations.activeDrive("ses_1"); got != nil {
		t.Fatalf("drive after owner exit = %p, want released", got)
	}
}

func TestGoalDriveAwaitReturnsCompletedOutcomeAfterCallerCancellation(t *testing.T) {
	want := errors.New("driver failed")
	drive := completedGoalDrive(want)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := drive.await(ctx); !errors.Is(err, want) {
		t.Fatalf("await() = %v, want completed outcome", err)
	}
}

func TestGoalRunCleanupAcceptsAnAlreadyTerminalRun(t *testing.T) {
	driver := &Driver{runs: alreadyTerminalRuns{}}

	if err := driver.cancelRun(t.Context(), "run_1"); err != nil {
		t.Fatalf("cancelRun after terminal commit: %v", err)
	}
}

func completedGoalDrive(err error) *goalDrive {
	done := make(chan struct{})
	close(done)
	return &goalDrive{
		done:   done,
		err:    err,
		cancel: func() {},
	}
}
