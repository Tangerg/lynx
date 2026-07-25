package turn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

func TestShutdownIsBoundedAndCanFinishJoiningLater(t *testing.T) {
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	err := shutdownDispatcher(ctx, dispatcher)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("shutdown error = %v, want ErrShutdownTimeout", err)
	}
	if !dispatcher.isClosed() {
		t.Fatal("timed-out shutdown did not reject future admission")
	}

	close(st.done)
	if err := shutdownDispatcher(t.Context(), dispatcher); err != nil {
		t.Fatalf("second shutdown after teardown = %v, want nil", err)
	}
}

func TestShutdownDeadlineCoversCancellationWork(t *testing.T) {
	release := make(chan struct{})
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	st.setProcess(&blockingCancelProcess{release: release})
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- shutdownDispatcher(ctx, dispatcher) }()

	select {
	case err := <-result:
		if !errors.Is(err, ErrShutdownTimeout) {
			t.Fatalf("shutdown error = %v, want ErrShutdownTimeout", err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("shutdown waited for blocking cancellation past its deadline")
	}

	close(release)
	select {
	case <-st.done:
	case <-time.After(time.Second):
		t.Fatal("turn did not finish after cancellation was released")
	}
}

type blockingCancelProcess struct {
	release         <-chan struct{}
	err             error
	discardErr      error
	discardReleased bool
}

func (*blockingCancelProcess) ID() string                 { return "proc_1" }
func (*blockingCancelProcess) Status() core.ProcessStatus { return core.StatusWaiting }
func (*blockingCancelProcess) Done() <-chan error         { return nil }
func (*blockingCancelProcess) Output() (agentexec.TurnOutput, error) {
	return agentexec.TurnOutput{}, nil
}
func (p *blockingCancelProcess) Cancel(context.Context) error {
	<-p.release
	return p.err
}
func (*blockingCancelProcess) Resume(context.Context, interrupts.Resolution) (<-chan error, error) {
	return nil, nil
}
func (*blockingCancelProcess) Suspension() *agent.Suspension { return nil }
func (p *blockingCancelProcess) Discard(context.Context) (bool, error) {
	return p.discardErr == nil || p.discardReleased, p.discardErr
}

func TestShutdownReportsProcessCancellationFailure(t *testing.T) {
	cancelErr := errors.New("kill failed")
	release := make(chan struct{})
	close(release)
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	process := &blockingCancelProcess{release: release, err: cancelErr}
	st.setProcess(process)
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	err := shutdownDispatcher(t.Context(), dispatcher)
	if !errors.Is(err, cancelErr) {
		t.Fatalf("shutdown error = %v, want process cancellation failure", err)
	}
	if channelClosed(st.done) {
		t.Fatal("failed cancellation released the turn")
	}
	if _, err := dispatcher.findTurn(st.handle.TurnID); err != nil {
		t.Fatalf("failed cancellation lost retry ownership: %v", err)
	}

	process.err = nil
	if err := shutdownDispatcher(t.Context(), dispatcher); err != nil {
		t.Fatalf("retry shutdown after transient cancellation failure: %v", err)
	}
	if !channelClosed(st.done) {
		t.Fatal("successful retry did not release the turn")
	}
}

func TestCancelRetainsTerminalTurnUntilDiscardSucceeds(t *testing.T) {
	discardErr := errors.New("discard failed")
	release := make(chan struct{})
	close(release)
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	process := &blockingCancelProcess{release: release, discardErr: discardErr}
	st.setProcess(process)
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	if err := dispatcher.Cancel(t.Context(), st.handle); !errors.Is(err, discardErr) {
		t.Fatalf("Cancel error = %v, want discard failure", err)
	}
	if channelClosed(st.done) {
		t.Fatal("discard failure released the turn")
	}
	if _, err := dispatcher.findTurn(st.handle.TurnID); err != nil {
		t.Fatalf("discard failure lost retry ownership: %v", err)
	}

	process.discardErr = nil
	if err := dispatcher.Cancel(t.Context(), st.handle); err != nil {
		t.Fatalf("retry terminal cleanup: %v", err)
	}
	if !channelClosed(st.done) {
		t.Fatal("successful terminal cleanup did not release the turn")
	}
}

func TestCancelReleasesTerminalTurnWhenDiscardCompletesWithDiagnostic(t *testing.T) {
	diagnostic := errors.New("process terminated with cleanup warning")
	release := make(chan struct{})
	close(release)
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	process := &blockingCancelProcess{
		release: release, discardErr: diagnostic, discardReleased: true,
	}
	st.setProcess(process)
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	if err := dispatcher.Cancel(t.Context(), st.handle); !errors.Is(err, diagnostic) {
		t.Fatalf("Cancel error = %v, want diagnostic", err)
	}
	if !channelClosed(st.done) {
		t.Fatal("completed discard did not release the turn")
	}
	if _, err := dispatcher.findTurn(st.handle.TurnID); !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("released turn remains registered: %v", err)
	}
}

func TestShutdownTimeoutKeepsLaterCancellationFailures(t *testing.T) {
	stalled := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_a"})
	stalledRelease := make(chan struct{})
	stalled.setProcess(&blockingCancelProcess{release: stalledRelease})
	if !stalled.parkIfLive() {
		t.Fatal("failed to park stalled turn")
	}
	cancelErr := errors.New("later cancellation failed")
	release := make(chan struct{})
	close(release)
	failed := newTurnState(t.Context(), TurnHandle{SessionID: "ses_2", TurnID: "turn_b"})
	failed.setProcess(&blockingCancelProcess{release: release, err: cancelErr})
	if !failed.parkIfLive() {
		t.Fatal("failed to park cancellation-error turn")
	}
	dispatcher := &memoryDispatcher{
		turns: map[string]*turnState{
			stalled.handle.TurnID: stalled,
			failed.handle.TurnID:  failed,
		},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithCancel(t.Context())
	dispatcher.BeginShutdown()
	for _, target := range dispatcher.shutdownTargets {
		target.attempt(ctx, dispatcher)
	}
	result := make(chan error, 1)
	go func() { result <- dispatcher.AwaitShutdown(ctx) }()
	var failedCancelDone <-chan struct{}
	for _, target := range dispatcher.shutdownTargets {
		if target.state != failed {
			continue
		}
		target.mu.Lock()
		if target.active != nil {
			failedCancelDone = target.active.done
		} else if target.last != nil {
			failedCancelDone = target.last.done
		}
		target.mu.Unlock()
		break
	}
	if failedCancelDone == nil {
		t.Fatal("failed turn has no shutdown cancellation attempt")
	}
	select {
	case <-failedCancelDone:
	case <-time.After(time.Second):
		t.Fatal("later cancellation did not finish")
	}
	cancel()
	err := <-result
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("shutdown error = %v, want ErrShutdownTimeout", err)
	}
	if !errors.Is(err, cancelErr) {
		t.Fatalf("shutdown error = %v, want completed cancellation failure", err)
	}

	close(stalledRelease)
	select {
	case <-stalled.done:
	case <-time.After(time.Second):
		t.Fatal("stalled cancellation did not finish after release")
	}
}

func shutdownDispatcher(ctx context.Context, dispatcher *memoryDispatcher) error {
	dispatcher.BeginShutdown()
	return dispatcher.AwaitShutdown(ctx)
}
