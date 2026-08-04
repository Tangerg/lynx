package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// Resume claims the parked run's session, prepares or rehydrates its turn,
// attaches and durably accepts a continuation segment, and only then activates
// the user's resolution.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (StartResult, error) {
	if err := c.requireUseCaseDependencies(); err != nil {
		return StartResult{}, err
	}
	pending, found, err := c.sessions.GetOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	if err := pending.Validate(); err != nil {
		return StartResult{}, fmt.Errorf("runs: invalid pending interrupt set: %w", err)
	}
	if gap := pending.Capabilities.MissingFrom(cmd.CallerCapabilities); !gap.IsEmpty() {
		return StartResult{}, &execution.InsufficientCapabilities{RunID: cmd.RunID, Missing: gap}
	}
	answers, err := resolveResumeResponses(pending, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	sess, err := c.sessions.Get(ctx, pending.SessionID)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, ok := c.admission.AcquireRun(pending.SessionID, sess.Cwd)
	if !ok {
		return StartResult{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", ErrSessionBusy, pending.SessionID, sess.Cwd)
	}
	defer runAdmission.Release()
	if c.runs == nil {
		return StartResult{}, errors.New("runs: run projection is required")
	}
	parkedRuns, err := c.runs.RunTree(ctx, pending.RootRunID)
	if err != nil {
		return StartResult{}, err
	}
	if err := validatePendingRunTree(pending, parkedRuns); err != nil {
		return StartResult{}, err
	}

	// Resume inherits the copy cwd + isolation from the parked turn's Runtime
	// scope, so no execution-cwd resolution is needed here. A rehydrate (process
	// gone) of an isolated run is refused as lost — see prepareTurn — because the
	// sandbox copy died with the process.
	turn, err := c.prepareTurn(ctx, pending, sess.Cwd, sess.Isolated)
	if err != nil {
		if errors.Is(err, ErrTurnStateLost) {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
			cleanupErr := c.sessions.ApplyRunLost(cleanupCtx, pending.SessionID, cmd.RunID, c.now().UTC())
			cancel()
			if cleanupErr != nil {
				return StartResult{}, errors.Join(err, fmt.Errorf("runs: recover lost run %q: %w", cmd.RunID, cleanupErr))
			}
			return StartResult{}, fmt.Errorf("%w: %w", ErrRunNotFound, err)
		}
		return StartResult{}, err
	}
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		return StartResult{}, errors.New("runs: pending interrupt set has no root continuation")
	}
	segmentID := c.newSegmentID()
	createdAt := rootContinuation.RunCreatedAt
	pendingCopy := pending
	continuation, err := treeContinuationFromPending(pendingCopy)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:          cmd.RunID,
		SegmentID:      segmentID,
		SessionID:      pending.SessionID,
		Cwd:            sess.Cwd,
		TurnID:         turn.TurnID,
		ModelSelection: rootContinuation.ModelSelection,
		GoalLeaseID:    pending.GoalLeaseID,
		CreatedAt:      createdAt,
		Input:          cmd.Input,
		Continuation:   continuation,
		admission:      &runAdmission,
		Activate: func(activateCtx context.Context) error {
			// The RUN's frozen kinds, not this request's: the caller has already been
			// checked to cover them, and taking the declaration here would let each
			// resume change what the next segment may park on.
			return c.turns.Resume(activateCtx, turn, answers, pending.Capabilities.InterruptKinds)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	// The continuation is durably accepted, which consumed the whole open set: the
	// run is running again and nothing in this session is waiting on a person.
	for _, continuation := range pending.Continuations {
		if continuation.RunID == pending.RootRunID {
			continue
		}
		c.publishRunMoved(pending.SessionID, continuation.RunID)
	}
	c.publishWaitingMoved(pending.SessionID, pending.RootRunID)
	result := StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: pending.SessionID, Events: events}
	if len(cmd.Input) > 0 {
		// Named only when there is an item to name: the id is derived from the segment
		// the same way a fresh run derives it, so the client reconciles its optimistic
		// bubble by id rather than by content.
		result.UserItemID = userMessageItemID(segmentID)
	}
	return result, nil
}

func (c *Coordinator) prepareTurn(ctx context.Context, pending interrupts.Pending, cwd string, isolated bool) (execution.TurnRef, error) {
	turn, err := c.turns.Prepare(ctx, execution.TurnRef{SessionID: pending.SessionID, TurnID: pending.TurnID})
	if err == nil {
		if err := turn.ValidateFor(pending.SessionID); err != nil {
			return execution.TurnRef{}, err
		}
		return turn, nil
	}
	if errors.Is(err, ErrParkClaimed) {
		return execution.TurnRef{}, ErrInterruptNotOpen
	}
	if !errors.Is(err, ErrTurnNotLive) {
		return execution.TurnRef{}, err
	}
	// The parked turn is not live in this process, so its executor died — for an
	// isolated run that means its sandbox copy, which lives only in this process's
	// Isolator, died with it. Rehydrating would rebuild the turn against the
	// project directory (the only cwd we still have), running the resumed model
	// and its memory extraction on the REAL tree — the exact pollution isolation
	// exists to prevent. Fail closed: the run's world is gone, so it is lost, not
	// resumable. Reusing ErrTurnStateLost routes it through the same durable
	// lost-run cleanup as a missing executor checkpoint.
	if isolated {
		return execution.TurnRef{}, fmt.Errorf("%w: an isolated run cannot resume after its sandbox process ended", ErrTurnStateLost)
	}
	root, ok := pending.RootContinuation()
	if !ok {
		return execution.TurnRef{}, errors.Join(
			ErrRunNotFound,
			errors.New("runs: interrupt has no root continuation"),
		)
	}
	turn, err = c.turns.Rehydrate(ctx, RehydrateTurn{
		SessionID:                pending.SessionID,
		TurnID:                   pending.TurnID,
		ProcessID:                root.ProcessID,
		RootRunID:                pending.RootRunID,
		ChildRuns:                childRunBindingsFromPending(pending),
		ModelSelection:           root.ModelSelection,
		Cwd:                      cwd,
		WorkspaceCwd:             cwd,
		Isolated:                 isolated,
		GoalLeaseID:              pending.GoalLeaseID,
		Limits:                   root.Limits,
		ChildRunAdmissionEnabled: pending.Capabilities.ChildRuns,
	})
	if err != nil {
		return execution.TurnRef{}, errors.Join(ErrRunNotFound, err)
	}
	if err := turn.ValidateFor(pending.SessionID); err != nil {
		return execution.TurnRef{}, err
	}
	return turn, nil
}

func childRunBindingsFromPending(pending interrupts.Pending) []ChildRunBinding {
	bindings := make([]ChildRunBinding, 0, len(pending.Continuations)-1)
	for _, continuation := range pending.Continuations {
		if !continuation.Lineage.IsChild() {
			continue
		}
		bindings = append(bindings, ChildRunBinding{
			ProcessID:   continuation.ProcessID,
			RunID:       continuation.RunID,
			ParentRunID: continuation.Lineage.ParentRunID,
		})
	}
	return bindings
}
