package turn

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type closeOnRestoreEngine struct {
	controller *controller
	process    agentexec.TurnProcess
}

func (*closeOnRestoreEngine) StartTurn(context.Context, agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	return nil, errors.New("unexpected StartTurn")
}

func (*closeOnRestoreEngine) SubagentProjection(string) (agentexec.SubagentProjection, bool) {
	return agentexec.SubagentProjection{}, false
}

func (e *closeOnRestoreEngine) RestoreTurn(context.Context, string, agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error) {
	e.controller.BeginShutdown()
	return e.process, nil
}

type gatedRestoreEngine struct {
	entered chan struct{}
	release chan struct{}
	process agentexec.TurnProcess
}

func (*gatedRestoreEngine) StartTurn(context.Context, agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	return nil, errors.New("unexpected StartTurn")
}

func (*gatedRestoreEngine) SubagentProjection(string) (agentexec.SubagentProjection, bool) {
	return agentexec.SubagentProjection{}, false
}

func (e *gatedRestoreEngine) RestoreTurn(context.Context, string, agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error) {
	close(e.entered)
	<-e.release
	return e.process, nil
}

func TestRehydrateCloseRaceReportsCleanupFailureAfterRelease(t *testing.T) {
	discardErr := errors.New("restored process discard failed")
	release := make(chan struct{})
	close(release)
	process := &blockingCancelProcess{release: release, discardErr: discardErr}
	engine := &closeOnRestoreEngine{
		process: process,
	}
	controller := &controller{
		engine:       engine,
		turns:        map[string]*turnState{},
		seenSessions: map[string]struct{}{},
	}
	engine.controller = controller

	_, err := controller.Rehydrate(t.Context(), runs.RehydrateExecution{
		SessionID:  "ses_1",
		ExecutorID: "turn_1",
		MemberID:   "proc_1",
		RootRunID:  "run_1",
	})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Rehydrate error = %v, want controller-close", err)
	}
	if err := controller.AwaitShutdown(t.Context()); !errors.Is(err, discardErr) {
		t.Fatalf("join failed shutdown cleanup = %v, want discard failure", err)
	}
	if _, err := controller.findTurn("turn_1"); err != nil {
		t.Fatalf("failed restored-process cleanup lost turn: %v", err)
	}

	process.discardErr = nil
	if err := controller.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("retry restored-process cleanup: %v", err)
	}
	if _, err := controller.findTurn("turn_1"); !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("successful restored-process cleanup retained turn: %v", err)
	}
}

func TestShutdownReleasesLateRestoredProcessAfterCleanupFailure(t *testing.T) {
	discardErr := errors.New("restored process discard failed")
	processRelease := make(chan struct{})
	close(processRelease)
	process := &blockingCancelProcess{release: processRelease, discardErr: discardErr}
	engine := &gatedRestoreEngine{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		process: process,
	}
	controller := &controller{
		engine:       engine,
		turns:        map[string]*turnState{},
		seenSessions: map[string]struct{}{},
	}

	rehydrated := make(chan error, 1)
	go func() {
		_, err := controller.Rehydrate(t.Context(), runs.RehydrateExecution{
			SessionID:  "ses_1",
			ExecutorID: "turn_1",
			MemberID:   "proc_1",
			RootRunID:  "run_1",
		})
		rehydrated <- err
	}()
	<-engine.entered

	controller.BeginShutdown()
	target := controller.shutdownTargets[0]
	lifecycleChanged := target.state.lifecycleChange()
	attempt := target.step.Begin(t.Context())
	// The first shutdown cancellation must finish its no-process transition
	// before Restore publishes the process; this is the interleaving that the old
	// one-shot Cancel result lost.
	<-lifecycleChanged
	close(engine.release)

	if err := <-rehydrated; !errors.Is(err, ErrClosed) {
		t.Fatalf("Rehydrate error = %v, want controller closed", err)
	}
	if err := attempt.Wait(t.Context()); !errors.Is(err, discardErr) {
		t.Fatalf("late-publication shutdown = %v, want discard failure", err)
	}
	if _, err := controller.findTurn("turn_1"); err != nil {
		t.Fatalf("failed late cleanup lost turn: %v", err)
	}

	process.discardErr = nil
	if err := controller.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("retry late cleanup: %v", err)
	}
	if _, err := controller.findTurn("turn_1"); !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("successful late cleanup retained turn: %v", err)
	}
}
