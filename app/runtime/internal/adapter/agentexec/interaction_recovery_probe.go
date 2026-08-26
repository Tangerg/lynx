package agentexec

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// CanResumeWaitingExecution probes one exact durable waiting tree without
// publishing or registering a live executor. Invalid, incompatible, or
// unknown-effect state returns false; assembly/probe I/O failures remain errors
// so startup never mutates facts after an inconclusive read. It deliberately
// executes the same restoration and product-member rebinding used by resume.
func (i *InteractionExecutor) CanResumeWaitingExecution(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (bool, error) {
	if i == nil {
		return false, errors.New("agentexec: Interaction executor is nil")
	}
	if err := continuation.Validate(); err != nil {
		return false, nil
	}
	checkpoint := continuation.Checkpoint
	if checkpoint.BuildID != i.config.BuildID || checkpoint.Scope.Isolated {
		return false, nil
	}
	if err := i.validateRestoreScope(ctx, checkpoint.Scope); err != nil {
		return false, nil
	}
	state, err := decodeInteractionCheckpointPayload(checkpoint.Payload)
	if err != nil {
		return false, nil
	}
	rootID, err := agent.ParseProcessID(checkpoint.RootMemberID)
	if err != nil || state.tree.RootID() != rootID {
		return false, nil
	}
	snapshots := state.tree.ProcessSnapshots()
	if len(snapshots) == 0 || snapshots[0].ProcessID() != rootID ||
		!isInteractionWaitingBoundary(snapshots[0].Status()) {
		return false, nil
	}
	start := runs.RootExecutionStart{
		SessionID:                checkpoint.Scope.SessionID,
		CWD:                      checkpoint.Scope.CWD,
		WorkspaceCWD:             checkpoint.Scope.WorkspaceCWD,
		Isolated:                 checkpoint.Scope.Isolated,
		GoalIncarnationID:        checkpoint.Scope.GoalIncarnationID,
		ModelSelection:           checkpoint.ModelSelection,
		Limits:                   checkpoint.Limits,
		InterruptKinds:           continuation.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: continuation.ChildRunAdmissionEnabled,
		WorkingContext:           cloneChatMessages(state.instructions),
	}
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: continuation.ExecutorID}
	assembled, err := i.assembleInteraction(ctx, ref, start)
	if err != nil {
		return false, fmt.Errorf("agentexec: assemble Interaction checkpoint probe: %w", err)
	}
	process, err := assembled.engine.RestoreTree(ctx, assembled.deployment, state.tree)
	if err != nil {
		_ = assembled.engine.Close()
		return false, nil
	}
	defer discardRestoredInteraction(assembled, process)
	if initializeRestoredContinuationErr := assembled.initializeRestoredContinuation(
		process,
		continuation,
		state,
		interactionBoundaryWaiting,
	); initializeRestoredContinuationErr != nil {
		return false, nil
	}
	unknown, err := assembled.unknownEffectIDs(ctx)
	if err != nil {
		return false, fmt.Errorf("agentexec: inspect Interaction checkpoint effects: %w", err)
	}
	if len(unknown) > 0 {
		return false, nil
	}
	interruptions, err := assembled.pendingInterruptions(state.tree)
	if err != nil {
		return false, nil
	}
	if len(interruptions) == 0 {
		return false, nil
	}
	return true, nil
}

var _ runs.WaitingExecutionResumability = (*InteractionExecutor)(nil)
