package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// validateRecoveryParkedTree checks the complete durable hand-off barrier before
// boot keeps a Run tree resumable. Pending is root-owned, so its continuations
// must cover every non-terminal member exactly once.
//
// An impossible partial application write is corruption and fails startup. A
// missing or executor-incompatible checkpoint is an external-resource loss and
// returns resumable=false so recovery can mark the whole tree run_lost.
func validateRecoveryParkedTree(
	ctx context.Context,
	tree recoveryRunTree,
	pending Pending,
	sess session.Session,
	items []transcript.Item,
	store RecoveryStore,
	resumability WaitingExecutionResumability,
) (bool, error) {
	values := make([]run.Run, 0, len(tree.postorder))
	for _, runID := range tree.postorder {
		values = append(values, tree.runsByID[runID])
	}
	if err := pending.ValidateProjection(values, items); err != nil {
		return false, fmt.Errorf("runs: validate recovery Run tree %q: %w", tree.root.ID(), err)
	}

	rootContinuation, _ := pending.RootContinuation()
	// Isolated workspaces are process-local scratch copies and are deliberately
	// never snapshotted. A host restart therefore destroys the world this tree
	// was parked in even when its executor payload remains decodable.
	if sess.Isolated() {
		return false, nil
	}
	expected := ExecutorCheckpointExpectation{
		RootMemberID:      rootContinuation.MemberID,
		SessionID:         pending.SessionID,
		CWD:               sess.CWD(),
		WorkspaceCWD:      sess.CWD(),
		Isolated:          false,
		GoalIncarnationID: pending.GoalIncarnationID,
		ModelSelection:    rootContinuation.ModelSelection,
		Limits:            rootContinuation.Limits,
		Capabilities:      pending.Capabilities,
	}
	checkpoint, err := store.LoadExecutorCheckpoint(ctx, rootContinuation.MemberID)
	if errors.Is(err, ErrExecutorCheckpointNotFound) || errors.Is(err, ErrInvalidExecutorCheckpoint) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"runs: load executor checkpoint %q for recovery: %w",
			rootContinuation.MemberID,
			err,
		)
	}
	if err := checkpoint.ValidateFor(expected); err != nil {
		return false, nil
	}
	continuation, err := waitingContinuationFromPending(pending, checkpoint)
	if err != nil {
		return false, fmt.Errorf(
			"runs: build waiting continuation %q for recovery: %w",
			rootContinuation.MemberID,
			err,
		)
	}
	resumable, err := resumability.CanResumeWaitingExecution(ctx, continuation)
	if err != nil {
		return false, fmt.Errorf(
			"runs: probe waiting execution %q resumability: %w",
			rootContinuation.MemberID,
			err,
		)
	}
	return resumable, nil
}
