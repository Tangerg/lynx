package turn

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
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
func (s *controller) Cancel(ctx context.Context, handle Handle) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	state.lifecycleMu.Lock()
	defer state.lifecycleMu.Unlock()
	state.cancel()
	switch state.requestCancellation() {
	case cancelFinish:
		return s.finishTurnOwned(ctx, state, run.OutcomeCanceled)
	case cancelProcess:
		if process := state.process(); process != nil {
			if err := cancelTurnProcess(ctx, process); err != nil {
				return err
			}
			return s.finishTurnOwned(ctx, state, run.OutcomeCanceled)
		}
		return nil
	case cancelComplete:
		if state.released() {
			return ErrTurnNotFound
		}
		return s.cleanupTurnOwned(ctx, state)
	default:
		// A start/run/resume goroutine owns the terminal, or Restore has not yet
		// published its process. Shutdown waits on lifecycle changes and retries
		// when ownership becomes actionable.
		return nil
	}
}

// CancelSubtree terminates one descendant process tree without claiming the
// turn's root lifecycle. The root process handle is stable for the turn; its
// implementation proves target ownership before Agent Runtime mutates anything.
func (s *controller) CancelSubtree(
	ctx context.Context,
	handle Handle,
	processID string,
) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	state.lifecycleMu.Lock()
	defer state.lifecycleMu.Unlock()
	if state.released() {
		return ErrTurnNotFound
	}
	process := state.process()
	if process == nil {
		return fmt.Errorf(
			"turn: cancel process subtree %q: turn %q has no process",
			processID,
			handle.TurnID,
		)
	}
	if err := process.CancelSubtree(ctx, processID); err != nil {
		return fmt.Errorf(
			"turn: cancel process subtree %q in turn %q: %w",
			processID,
			handle.TurnID,
			err,
		)
	}
	return nil
}

// PrepareWaitingSubtreeCancellation claims one parked turn and asks the Agent
// adapter for an immutable execution-transition plan. The returned application
// capability owns only the Application lifecycle claim until Commit or Abort; Agent
// runtime retains no resource while the Application transaction runs.
func (s *controller) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	handle Handle,
	processID string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	state.lifecycleMu.Lock()
	defer state.lifecycleMu.Unlock()
	if state.released() {
		return runs.PreparedWaitingSubtreeCancellation{}, ErrTurnNotFound
	}
	if !state.claimWaitingMutation() {
		return runs.PreparedWaitingSubtreeCancellation{}, ErrParkClaimed
	}
	process := state.process()
	if process == nil {
		state.abortWaitingMutation()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"turn: prepare waiting process subtree %q: turn %q has no process",
			processID,
			handle.TurnID,
		)
	}
	planner, ok := process.(agentexec.WaitingSubtreePlanner)
	if !ok {
		state.abortWaitingMutation()
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New("turn: process does not support planning a waiting subtree cancellation")
	}
	plan, err := planner.PlanWaitingSubtreeCancellation(ctx, processID)
	if err != nil {
		state.abortWaitingMutation()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"turn: plan waiting process subtree %q in turn %q: %w",
			processID,
			handle.TurnID,
			err,
		)
	}
	suspensions := plan.PendingSuspensions()
	projected := make([]runs.MemberInterruption, len(suspensions))
	for index, boundary := range suspensions {
		interrupt, ok := typedInterrupt(boundary.Prompt)
		if !ok {
			state.abortWaitingMutation()
			return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
				"turn: planned process %q suspension %q has an unsupported interrupt payload",
				boundary.ProcessID,
				boundary.SuspensionID,
			)
		}
		projected[index] = runs.MemberInterruption{
			MemberID:  boundary.ProcessID,
			RequestID: boundary.SuspensionID,
			Interrupt: interrupt,
		}
	}
	mutation := &preparedWaitingSubtreeCancellation{controller: s, state: state, plan: plan}
	return runs.PreparedWaitingSubtreeCancellation{
		CanceledMemberIDs:    plan.CanceledProcessIDs(),
		PendingInterruptions: projected,
		Checkpoint:           plan.Checkpoint(),
		Mutation:             mutation,
	}, nil
}

type preparedWaitingSubtreeCancellation struct {
	mu sync.Mutex

	controller *controller
	state      *turnState
	plan       agentexec.WaitingSubtreeCancellationPlan
	settled    bool
}

func (prepared *preparedWaitingSubtreeCancellation) Commit(
	ctx context.Context,
	disposition runs.WaitingSubtreeDisposition,
) error {
	if prepared == nil || prepared.state == nil || prepared.plan == nil {
		return errors.New("turn: commit waiting subtree cancellation: incomplete application operation")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.settled {
		return nil
	}
	switch disposition {
	case runs.WaitingSubtreeRemainsInterrupted, runs.WaitingSubtreeContinues:
	default:
		return fmt.Errorf("turn: unknown waiting subtree disposition %d", disposition)
	}

	prepared.state.lifecycleMu.Lock()
	if err := prepared.plan.Apply(ctx); err != nil {
		prepared.state.lifecycleMu.Unlock()
		return fmt.Errorf(
			"turn: apply waiting subtree runtime transition for turn %q: %w",
			prepared.state.handle.TurnID,
			err,
		)
	}
	prepared.settled = true
	if disposition == runs.WaitingSubtreeRemainsInterrupted {
		prepared.state.commitWaitingMutation(false)
		prepared.state.lifecycleMu.Unlock()
		return nil
	}
	if err := prepared.plan.Continue(prepared.state.ctx); err != nil {
		continuationErr := fmt.Errorf(
			"turn: continue applied waiting subtree transition for turn %q: %w",
			prepared.state.handle.TurnID,
			err,
		)
		prepared.state.commitWaitingMutation(true)
		prepared.state.lifecycleMu.Unlock()
		finishErr := prepared.controller.finishExecutionError(
			prepared.state,
			problemFromError(continuationErr),
			continuationErr,
		)
		return errors.Join(continuationErr, finishErr)
	}
	if !prepared.state.commitWaitingMutation(true) {
		// Shutdown claimed the turn while the durable transaction committed.
		// Its lifecycle owner will cancel the now-stable runtime tree.
		prepared.state.lifecycleMu.Unlock()
		return nil
	}
	prepared.state.lifecycleMu.Unlock()
	go prepared.controller.drive(prepared.state)
	return nil
}

func (prepared *preparedWaitingSubtreeCancellation) Abort() {
	if prepared == nil || prepared.state == nil || prepared.plan == nil {
		return
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.settled {
		return
	}
	prepared.state.lifecycleMu.Lock()
	prepared.state.abortWaitingMutation()
	prepared.state.lifecycleMu.Unlock()
	prepared.settled = true
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
func (s *controller) Resume(ctx context.Context, handle Handle, answers []agentexec.SuspensionAnswer, interruptKinds []interrupt.Kind) error {
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
	return s.resumeAndDrive(ctx, state, answers)
}

// resumeAndDrive delivers the decision to the turn's (write-once-stable) parked
// process and launches the continuation drive. On a resume error it streams the
// terminal TurnEnd and returns the error; otherwise it starts
// drive and returns nil. Shared by [Resume] (same-process) and [Rehydrate]
// (cross-restart) so the resume tail — deliver, on-error-finish, else-drive —
// stays identical.
func (s *controller) resumeAndDrive(
	admissionCtx context.Context,
	state *turnState,
	answers []agentexec.SuspensionAnswer,
) error {
	err := state.process().Resume(admissionCtx, answers)
	if err != nil {
		if (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) &&
			state.resumeAdmissionFailed() {
			return err
		}
		cancelErr := cancelTurnProcess(state.ctx, state.process())
		finishErr := s.finishExecutionError(state, problemFromError(err), err)
		return errors.Join(err, cancelErr, finishErr)
	}
	state.resumeStarted()
	go s.drive(state)
	return nil
}
