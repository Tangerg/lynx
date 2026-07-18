package turn

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

type closeOnRestoreEngine struct {
	dispatcher *memoryDispatcher
	process    agentexec.TurnProcess
}

func (*closeOnRestoreEngine) StartTurn(context.Context, agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	return nil, errors.New("unexpected StartTurn")
}

func (e *closeOnRestoreEngine) RestoreTurn(context.Context, string, agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error) {
	if err := e.dispatcher.close(context.Background()); err != nil {
		return nil, err
	}
	return e.process, nil
}

func TestRehydratePreservesCloseRaceCleanupFailure(t *testing.T) {
	cancelErr := errors.New("restored process cleanup failed")
	discardErr := errors.New("restored process discard failed")
	release := make(chan struct{})
	close(release)
	engine := &closeOnRestoreEngine{
		process: &blockingCancelProcess{release: release, err: cancelErr, discardErr: discardErr},
	}
	dispatcher := &memoryDispatcher{
		engine:       engine,
		turns:        map[string]*turnState{},
		seenSessions: map[string]struct{}{},
	}
	engine.dispatcher = dispatcher

	_, err := dispatcher.Rehydrate(t.Context(), RehydrateRequest{
		SessionID: "ses_1",
		TurnID:    "turn_1",
		ProcessID: "proc_1",
	})
	if !errors.Is(err, ErrDispatcherClosed) || !errors.Is(err, cancelErr) || !errors.Is(err, discardErr) {
		t.Fatalf("Rehydrate error = %v, want dispatcher-close, cancel, and discard failures", err)
	}
}
