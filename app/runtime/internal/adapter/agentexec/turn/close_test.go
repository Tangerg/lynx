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

func TestCloseIsBoundedAndCanFinishJoiningLater(t *testing.T) {
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	err := dispatcher.close(ctx)
	if !errors.Is(err, ErrCloseTimeout) {
		t.Fatalf("close error = %v, want ErrCloseTimeout", err)
	}
	if !dispatcher.isClosed() {
		t.Fatal("timed-out close did not reject future admission")
	}

	close(st.done)
	if err := dispatcher.close(t.Context()); err != nil {
		t.Fatalf("second close after teardown = %v, want nil", err)
	}
}

func TestCloseDeadlineCoversCancellationWork(t *testing.T) {
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
	go func() { result <- dispatcher.close(ctx) }()

	select {
	case err := <-result:
		if !errors.Is(err, ErrCloseTimeout) {
			t.Fatalf("close error = %v, want ErrCloseTimeout", err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("close waited for blocking cancellation past its deadline")
	}

	close(release)
	select {
	case <-st.done:
	case <-time.After(time.Second):
		t.Fatal("turn did not finish after cancellation was released")
	}
}

type blockingCancelProcess struct {
	release <-chan struct{}
}

func (*blockingCancelProcess) ID() string                 { return "proc_1" }
func (*blockingCancelProcess) Status() core.ProcessStatus { return core.StatusWaiting }
func (*blockingCancelProcess) Done() <-chan error         { return nil }
func (*blockingCancelProcess) Output() (agentexec.TurnOutput, error) {
	return agentexec.TurnOutput{}, nil
}
func (p *blockingCancelProcess) Cancel() error {
	<-p.release
	return nil
}
func (*blockingCancelProcess) Resume(context.Context, interrupts.Resolution) (<-chan error, error) {
	return nil, nil
}
func (*blockingCancelProcess) Suspension() *agent.Suspension { return nil }
func (*blockingCancelProcess) Discard(context.Context)       {}
