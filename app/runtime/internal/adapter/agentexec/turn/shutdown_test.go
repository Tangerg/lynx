package turn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// hangBound separates a hang from a slow machine, which is all these waits have
// to do. Each one guards a failure that would block forever — shutdown waiting
// on a cancellation that is never released, or a turn that never reaches its
// terminal — so none of them is asserting a duration. Keeping it generous means
// a loaded machine cannot fail the suite, while Go's own test timeout still
// backstops a genuine deadlock.
const hangBound = 30 * time.Second

func TestShutdownIsBoundedAndCanFinishJoiningLater(t *testing.T) {
	st := newRestoringTurnState(t.Context(), Handle{SessionID: "ses_1", TurnID: "turn_1"})
	controller := &controller{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	err := shutdownController(ctx, controller)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("shutdown error = %v, want ErrShutdownTimeout", err)
	}
	if !controller.isClosed() {
		t.Fatal("timed-out shutdown did not reject future admission")
	}

	close(st.done)
	if err := shutdownController(t.Context(), controller); err != nil {
		t.Fatalf("second shutdown after teardown = %v, want nil", err)
	}
}

func TestShutdownDeadlineCoversCancellationWork(t *testing.T) {
	release := make(chan struct{})
	st := newRunningTestState(
		t.Context(),
		Handle{SessionID: "ses_1", TurnID: "turn_1"},
		&blockingCancelProcess{release: release},
	)
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	controller := &controller{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- shutdownController(ctx, controller) }()

	select {
	case err := <-result:
		if !errors.Is(err, ErrShutdownTimeout) {
			t.Fatalf("shutdown error = %v, want ErrShutdownTimeout", err)
		}
	case <-time.After(hangBound):
		close(release)
		t.Fatal("shutdown waited for blocking cancellation past its deadline")
	}

	close(release)
	select {
	case <-st.done:
	case <-time.After(hangBound):
		t.Fatal("turn did not finish after cancellation was released")
	}
}

type blockingCancelProcess struct {
	release    <-chan struct{}
	err        error
	discardErr error
	discarded  chan struct{}
}

func (*blockingCancelProcess) ID() string { return "proc_1" }
func (*blockingCancelProcess) Await() agentexec.TurnCompletion {
	return agentexec.TurnCompletion{Status: core.StatusWaiting}
}
func (p *blockingCancelProcess) Cancel(context.Context) error {
	<-p.release
	return p.err
}
func (*blockingCancelProcess) CancelSubtree(context.Context, string) error                { return nil }
func (*blockingCancelProcess) Resume(context.Context, []agentexec.SuspensionAnswer) error { return nil }
func (*blockingCancelProcess) PendingSuspensions(context.Context) ([]agentexec.PendingSuspension, error) {
	return nil, nil
}
func (*blockingCancelProcess) CaptureWaitingCheckpoint(context.Context) (agentexec.WaitingCheckpoint, error) {
	return testWaitingCheckpointValue(), nil
}
func (p *blockingCancelProcess) Discard(context.Context) error {
	err := p.discardErr
	if p.discarded != nil {
		p.discarded <- struct{}{}
	}
	return err
}

func testWaitingCheckpointValue() agentexec.WaitingCheckpoint {
	return agentexec.WaitingCheckpoint{Checkpoint: execution.ExecutorCheckpoint{
		RootProcessID: "process_root",
		Payload:       []byte(`{"root":"process_root"}`),
		BuildID:       "build",
	}}
}

func TestShutdownReportsProcessCancellationFailure(t *testing.T) {
	cancelErr := errors.New("kill failed")
	release := make(chan struct{})
	close(release)
	process := &blockingCancelProcess{release: release, err: cancelErr}
	st := newRunningTestState(t.Context(), Handle{SessionID: "ses_1", TurnID: "turn_1"}, process)
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	controller := &controller{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	err := shutdownController(t.Context(), controller)
	if !errors.Is(err, cancelErr) {
		t.Fatalf("shutdown error = %v, want process cancellation failure", err)
	}
	if channelClosed(st.done) {
		t.Fatal("failed cancellation released the turn")
	}
	if _, err := controller.findTurn(st.handle.TurnID); err != nil {
		t.Fatalf("failed cancellation lost retry ownership: %v", err)
	}

	process.err = nil
	if err := shutdownController(t.Context(), controller); err != nil {
		t.Fatalf("retry shutdown after transient cancellation failure: %v", err)
	}
	if !channelClosed(st.done) {
		t.Fatal("successful retry did not release the turn")
	}
}

func TestCancelRetainsTerminalTurnAfterBackgroundDiscardFailure(t *testing.T) {
	discardErr := errors.New("discard failed")
	release := make(chan struct{})
	close(release)
	process := &blockingCancelProcess{
		release:    release,
		discardErr: discardErr,
		discarded:  make(chan struct{}, 2),
	}
	st := newRunningTestState(t.Context(), Handle{SessionID: "ses_1", TurnID: "turn_1"}, process)
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	controller := &controller{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	if err := controller.Cancel(t.Context(), st.handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	<-process.discarded
	if channelClosed(st.done) {
		t.Fatal("discard failure released the terminal turn")
	}
	if _, err := controller.findTurn(st.handle.TurnID); err != nil {
		t.Fatalf("discard failure lost registry ownership: %v", err)
	}
	process.discardErr = nil
	if err := controller.Cancel(t.Context(), st.handle); err != nil {
		t.Fatalf("retry terminal cleanup: %v", err)
	}
	if !channelClosed(st.done) {
		t.Fatal("successful retry retained the terminal turn")
	}
}

func TestShutdownTimeoutKeepsLaterCancellationFailures(t *testing.T) {
	stalledRelease := make(chan struct{})
	stalled := newRunningTestState(
		t.Context(),
		Handle{SessionID: "ses_1", TurnID: "turn_a"},
		&blockingCancelProcess{release: stalledRelease},
	)
	if !stalled.parkIfLive() {
		t.Fatal("failed to park stalled turn")
	}
	cancelErr := errors.New("later cancellation failed")
	release := make(chan struct{})
	close(release)
	failed := newRunningTestState(
		t.Context(),
		Handle{SessionID: "ses_2", TurnID: "turn_b"},
		&blockingCancelProcess{release: release, err: cancelErr},
	)
	if !failed.parkIfLive() {
		t.Fatal("failed to park cancellation-error turn")
	}
	controller := &controller{
		turns: map[string]*turnState{
			stalled.handle.TurnID: stalled,
			failed.handle.TurnID:  failed,
		},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err := shutdownController(ctx, controller)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("shutdown error = %v, want ErrShutdownTimeout", err)
	}
	if !errors.Is(err, cancelErr) {
		t.Fatalf("shutdown error = %v, want completed cancellation failure", err)
	}

	close(stalledRelease)
	select {
	case <-stalled.done:
	case <-time.After(hangBound):
		t.Fatal("stalled cancellation did not finish after release")
	}
}

func shutdownController(ctx context.Context, controller *controller) error {
	controller.BeginShutdown()
	return controller.AwaitShutdown(ctx)
}
