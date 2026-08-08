package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// Resume claims the parked Run's Session, prepares or rehydrates its executor,
// attaches and durably accepts a continuation segment, and only then activates
// the user's resolution.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (StartResult, error) {
	if err := c.requireUseCaseDependencies(); err != nil {
		return StartResult{}, err
	}
	pending, found, err := c.sessions.LookupOpenInterrupt(ctx, cmd.RunID)
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
		return StartResult{}, &run.InsufficientCapabilities{RunID: cmd.RunID, Missing: gap}
	}
	answers, err := resolveResumeResponses(pending, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	sess, err := c.sessions.Get(ctx, pending.SessionID)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, ok := c.admission.AcquireRun(pending.SessionID, sess.CWD)
	if !ok {
		return StartResult{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", ErrSessionBusy, pending.SessionID, sess.CWD)
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

	// Resume inherits the copy cwd and isolation from the parked execution scope,
	// so no execution-cwd resolution is needed here. A rehydrate (process
	// gone) of an isolated Run is refused as lost — see prepareExecution — because the
	// sandbox copy died with the process.
	ref, err := c.prepareExecution(ctx, pending, sess.CWD, sess.Isolated)
	if err != nil {
		if errors.Is(err, ErrExecutorStateLost) {
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
		CWD:            sess.CWD,
		ExecutorID:     ref.ExecutorID,
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
			return c.control.Resume(activateCtx, ref, answers, pending.Capabilities.InterruptKinds)
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

func (c *Coordinator) prepareExecution(ctx context.Context, pending Pending, cwd string, isolated bool) (ExecutorRef, error) {
	ref, err := c.control.Prepare(ctx, ExecutorRef{SessionID: pending.SessionID, ExecutorID: pending.ExecutorID})
	if err == nil {
		if err := ref.ValidateFor(pending.SessionID); err != nil {
			return ExecutorRef{}, err
		}
		return ref, nil
	}
	if errors.Is(err, ErrExecutionClaimed) {
		return ExecutorRef{}, ErrInterruptNotOpen
	}
	if !errors.Is(err, ErrExecutorNotLive) {
		return ExecutorRef{}, err
	}
	// The parked execution is not live in this process, so its executor died. For
	// an isolated Run that means its sandbox copy, which is process-local, died
	// with it. Rehydrating would rebuild execution against the
	// project directory (the only cwd we still have), running the resumed model
	// and its memory extraction on the REAL tree — the exact pollution isolation
	// exists to prevent. Fail closed: the run's world is gone, so it is lost, not
	// resumable. ErrExecutorStateLost routes it through the same durable
	// lost-run cleanup as a missing executor checkpoint.
	if isolated {
		return ExecutorRef{}, fmt.Errorf("%w: an isolated run cannot resume after its sandbox process ended", ErrExecutorStateLost)
	}
	root, ok := pending.RootContinuation()
	if !ok {
		return ExecutorRef{}, errors.Join(
			ErrRunNotFound,
			errors.New("runs: interrupt has no root continuation"),
		)
	}
	ref, err = c.control.Rehydrate(ctx, RehydrateExecution{
		SessionID:                pending.SessionID,
		ExecutorID:               pending.ExecutorID,
		ProcessID:                root.ProcessID,
		RootRunID:                pending.RootRunID,
		ChildRuns:                childRunBindingsFromPending(pending),
		ModelSelection:           root.ModelSelection,
		CWD:                      cwd,
		WorkspaceCWD:             cwd,
		Isolated:                 isolated,
		GoalLeaseID:              pending.GoalLeaseID,
		Limits:                   root.Limits,
		ChildRunAdmissionEnabled: pending.Capabilities.ChildRuns,
	})
	if err != nil {
		return ExecutorRef{}, errors.Join(ErrRunNotFound, err)
	}
	if err := ref.ValidateFor(pending.SessionID); err != nil {
		return ExecutorRef{}, err
	}
	return ref, nil
}

func childRunBindingsFromPending(pending Pending) []ChildRunBinding {
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
