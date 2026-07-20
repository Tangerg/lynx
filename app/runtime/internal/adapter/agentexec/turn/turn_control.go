package turn

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
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
func (s *memoryDispatcher) Cancel(_ context.Context, handle TurnHandle) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	state.cancel()
	if state.cancelPrepared() {
		s.finishTurn(state, execution.OutcomeCanceled)
		return nil
	}
	// Claim the parked flag so a racing Resume can't also act on the same
	// suspended turn (whoever flips it false wins).
	process := state.process()
	claimed := state.claimPark()
	if process != nil {
		status := process.Status()
		switch {
		case claimed && status != core.StatusRunning:
			// This Cancel owns the parked suspension. There is no continuation
			// loop to observe ctx cancellation, so terminate the whole tree.
			err = cancelTurnProcess(process)
		case !claimed && status != core.StatusRunning && status != core.StatusWaiting:
			// A not-yet-running, non-parked process also has no loop that can
			// observe ctx cancellation. Waiting is deliberately excluded: a
			// racing Resume may have won claimPark but not yet recorded the
			// response. Killing that transient Waiting process would clear its
			// suspension and make the winning Resume fail stale.
			err = cancelTurnProcess(process)
		}
	}
	if claimed {
		// The turn was parked on an interrupt — no drive goroutine is waiting on
		// it, so emit the terminal + tear down here.
		s.finishTurn(state, execution.OutcomeCanceled)
	}
	return err
}

func cancelTurnProcess(process agentexec.TurnProcess) error {
	if process == nil {
		return nil
	}
	if err := process.Cancel(); err != nil {
		return fmt.Errorf("turn: cancel process %q: %w", process.ID(), err)
	}
	return nil
}

// Resume answers a turn parked on a HITL interrupt (tool approval or plan
// review). It claims the parked flag (so a racing Cancel can't double-act),
// delivers the bool decision to the agent process, and drives the continuation
// segment onto the same event channel. Returns [ErrTurnNotFound] when the turn
// isn't parked (unknown / already resumed / terminal).
func (s *memoryDispatcher) Resume(_ context.Context, handle TurnHandle, resolution interrupts.Resolution, interruptKinds []string) error {
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
// terminal (ErrorEvent + TurnEnd) and returns the error; otherwise it starts
// drive and returns nil. Shared by [Resume] (same-process) and [Rehydrate]
// (cross-restart) so the resume tail — deliver, on-error-finish, else-drive —
// stays identical.
func (s *memoryDispatcher) resumeAndDrive(state *turnState, resolution interrupts.Resolution) error {
	resumed, err := state.process().Resume(state.ctx, resolution)
	if err != nil {
		s.emit(state, ErrorEvent{Message: err.Error(), Code: ErrorCodeEngine, Problem: problemFromError(err)})
		s.finishTurn(state, execution.OutcomeError)
		return err
	}
	go s.drive(state, resumed)
	return nil
}
