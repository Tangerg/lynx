package agentexec

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// CanResumeCheckpoint probes one durable, opaque waiting tree without
// publishing or registering a live executor. Invalid, missing, incompatible or
// unknown-effect checkpoints return false; storage/probe I/O failures remain
// errors so startup never mutates facts after an inconclusive read.
func (executor *InteractionExecutor) CanResumeCheckpoint(
	ctx context.Context,
	expected runs.ExecutorCheckpointExpectation,
) (bool, error) {
	if executor == nil {
		return false, errors.New("agentexec: native Interaction executor is nil")
	}
	if executor.config.Checkpoints == nil {
		return false, errors.New("agentexec: native Interaction checkpoint reader is unavailable")
	}
	checkpoint, err := executor.config.Checkpoints.LoadCheckpoint(ctx, expected.RootMemberID)
	if err != nil {
		if errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("agentexec: load Interaction checkpoint: %w", err)
	}
	if err := checkpoint.ValidateFor(expected); err != nil || checkpoint.BuildID != executor.config.BuildID || checkpoint.Scope.Isolated {
		return false, nil
	}
	if err := executor.validateRestoreScope(ctx, checkpoint.Scope); err != nil {
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
		GoalLeaseID:              checkpoint.Scope.GoalLeaseID,
		ModelSelection:           checkpoint.ModelSelection,
		Limits:                   checkpoint.Limits,
		InterruptKinds:           expected.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: expected.Capabilities.ChildRuns,
		WorkingContext:           cloneChatMessages(state.instructions),
	}
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: "checkpoint-probe"}
	assembled, err := executor.assembleInteraction(ctx, ref, start)
	if err != nil {
		return false, fmt.Errorf("agentexec: assemble Interaction checkpoint probe: %w", err)
	}
	process, err := assembled.engine.RestoreTree(ctx, assembled.deployment, state.tree)
	if err != nil {
		_ = assembled.engine.Close()
		return false, nil
	}
	defer discardRestoredInteraction(assembled, process)
	assembled.setProcess(process)
	unknown, err := assembled.unknownEffectIDs(ctx)
	if err != nil {
		return false, fmt.Errorf("agentexec: inspect Interaction checkpoint effects: %w", err)
	}
	if len(unknown) > 0 {
		return false, nil
	}
	interruptions, err := assembled.pendingInterruptions(state.tree)
	return err == nil && len(interruptions) > 0, nil
}

var _ runs.CheckpointResumability = (*InteractionExecutor)(nil)
