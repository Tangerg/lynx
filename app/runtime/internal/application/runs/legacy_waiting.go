package runs

import (
	"context"
	"errors"
	"fmt"
)

// prepareLegacyWaitingExecution belongs solely to the P7 waiting-subtree
// bridge. Ordinary resume uses WaitingExecutionContinuer and the answer-claim
// transaction; this helper is deleted with the old subtree controller in P8.
func (c *Coordinator) prepareLegacyWaitingExecution(
	ctx context.Context,
	pending Pending,
	cwd string,
	isolated bool,
) (ExecutorRef, error) {
	if c.legacyWaiting == nil {
		return ExecutorRef{}, errors.New("runs: legacy waiting executor is unavailable")
	}
	ref, err := c.legacyWaiting.ClaimWaiting(ctx, ExecutorRef{
		SessionID: pending.SessionID, ExecutorID: pending.ExecutorID,
	})
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
	if isolated {
		return ExecutorRef{}, fmt.Errorf(
			"%w: an isolated legacy execution cannot resume after its process ended",
			ErrExecutorStateLost,
		)
	}
	root, ok := pending.RootContinuation()
	if !ok {
		return ExecutorRef{}, errors.Join(ErrRunNotFound, errors.New("runs: interrupt has no root continuation"))
	}
	ref, err = c.legacyWaiting.RestoreWaiting(ctx, RehydrateExecution{
		SessionID: pending.SessionID, ExecutorID: pending.ExecutorID,
		MemberID: root.MemberID, RootRunID: pending.RootRunID,
		ChildRuns: childRunBindingsFromPending(pending), ModelSelection: root.ModelSelection,
		CWD: cwd, WorkspaceCWD: cwd, Isolated: isolated,
		GoalLeaseID: pending.GoalLeaseID, Limits: root.Limits,
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
