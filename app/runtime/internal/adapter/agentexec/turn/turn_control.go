package turn

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// Cancel stops a turn. The ctx cancel is the primary signal: it aborts any
// in-flight LLM stream (which reads ctx.Done()) and drives a RUNNING process's
// run loop to its own terminal via markCancelled — the single ProcessKilled
// publisher. Kill is reserved for a process that ISN'T looping
// (parked/suspended on a HITL interrupt, or not yet started): there's no loop
// to observe the ctx cancel, so it's terminated explicitly. Killing a Running
// process here instead would clobber its status — dropping a continuation a
// racing Resume just started (the approved tool never runs) — and publish a
// duplicate ProcessKilled alongside markCancelled.
func (s *memoryDispatcher) Cancel(ctx context.Context, handle TurnHandle) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	state.lifecycleMu.Lock()
	defer state.lifecycleMu.Unlock()
	state.cancel()
	switch state.requestCancellation() {
	case cancelFinish:
		return s.finishTurnOwned(state, execution.OutcomeCanceled)
	case cancelProcess:
		if process := state.process(); process != nil {
			if err := cancelTurnProcess(ctx, process); err != nil {
				return err
			}
			return s.finishTurnOwned(state, execution.OutcomeCanceled)
		}
		return nil
	case cancelCleanup:
		return s.cleanupTurnOwned(state)
	case cancelComplete:
		return ErrTurnNotFound
	default:
		// A start/run/resume goroutine owns the terminal, or Restore has not yet
		// published its process. Shutdown waits on lifecycle changes and retries
		// when ownership becomes actionable.
		return nil
	}
}

func cancelTurnProcess(ctx context.Context, process agentexec.TurnProcess) error {
	if process == nil {
		return nil
	}
	if err := process.Cancel(ctx); err != nil {
		return fmt.Errorf("turn: cancel process %q: %w", process.ID(), err)
	}
	return nil
}

// Resume answers a turn parked on a HITL interrupt (tool approval or plan
// review). It claims the parked flag (so a racing Cancel can't double-act),
// delivers the bool decision to the agent process, and drives the continuation
// segment onto the same event channel. Returns [ErrTurnNotFound] when the turn
// isn't parked (unknown / already resumed / terminal).
func (s *memoryDispatcher) Resume(_ context.Context, handle TurnHandle, resolution interrupts.Resolution, interruptKinds []runs.InterruptKind) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	if !state.claimPark() {
		// The turn exists but its park was already claimed — a concurrent Cancel
		// is finishing it. Report it distinctly from ErrTurnNotFound (turn gone /
		// restart) so the caller doesn't rehydrate and resurrect a canceled turn.
		return ErrParkClaimed
	}
	state.setInterruptKinds(interruptKinds)
	return s.resumeAndDrive(state, resolution)
}

// resumeAndDrive delivers the decision to the turn's (write-once-stable) parked
// process and launches the continuation drive. On a resume error it streams the
// terminal TurnEnd and returns the error; otherwise it starts
// drive and returns nil. Shared by [Resume] (same-process) and [Rehydrate]
// (cross-restart) so the resume tail — deliver, on-error-finish, else-drive —
// stays identical.
func (s *memoryDispatcher) resumeAndDrive(state *turnState, resolution interrupts.Resolution) error {
	if err := state.process().Resume(state.ctx, resolution); err != nil {
		return errors.Join(
			err,
			s.finishExecutionError(state, problemFromError(err), err),
		)
	}
	state.resumeStarted()
	go s.drive(state)
	return nil
}
