package turn

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type closeOnRestoreEngine struct {
	dispatcher *memoryDispatcher
	process    agentexec.TurnProcess
}

func (*closeOnRestoreEngine) StartTurn(context.Context, agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	return nil, errors.New("unexpected StartTurn")
}

func (e *closeOnRestoreEngine) RestoreTurn(context.Context, string, agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error) {
	e.dispatcher.BeginShutdown()
	return e.process, nil
}

func TestRehydrateCloseRaceRetainsFailedCleanupForShutdownRetry(t *testing.T) {
	discardErr := errors.New("restored process discard failed")
	release := make(chan struct{})
	close(release)
	process := &blockingCancelProcess{release: release, discardErr: discardErr}
	engine := &closeOnRestoreEngine{
		process: process,
	}
	dispatcher := &memoryDispatcher{
		engine:       engine,
		turns:        map[string]*turnState{},
		seenSessions: map[string]struct{}{},
	}
	engine.dispatcher = dispatcher

	_, err := dispatcher.Rehydrate(t.Context(), runs.RehydrateTurn{
		SessionID: "ses_1",
		TurnID:    "turn_1",
		ProcessID: "proc_1",
	})
	if !errors.Is(err, ErrDispatcherClosed) || !errors.Is(err, discardErr) {
		t.Fatalf("Rehydrate error = %v, want dispatcher-close and discard failure", err)
	}
	if err := dispatcher.AwaitShutdown(t.Context()); !errors.Is(err, discardErr) {
		t.Fatalf("join failed shutdown cleanup = %v, want discard failure", err)
	}
	if _, err := dispatcher.findTurn("turn_1"); err != nil {
		t.Fatalf("failed restored-process cleanup lost ownership: %v", err)
	}

	process.discardErr = nil
	if err := dispatcher.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("retry shutdown cleanup: %v", err)
	}
	if _, err := dispatcher.findTurn("turn_1"); !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("successful retry retained turn: %v", err)
	}
}
