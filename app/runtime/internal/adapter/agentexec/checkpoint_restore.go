package agentexec

import (
	"context"
	"errors"
	"fmt"

	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// ErrExecutorCheckpointLost reports that a parked process cannot be
// reconstructed from its durable checkpoint and the owning application Run
// must be recovered lost.
var ErrExecutorCheckpointLost = errors.New("agentexec: executor checkpoint lost")

// CanResumeCheckpoint reports whether expected names a compatible waiting
// continuation owned by this engine and the same application Run/Session.
func (e *Engine) CanResumeCheckpoint(
	ctx context.Context,
	expected execution.ExecutorCheckpointExpectation,
) (bool, error) {
	if e == nil || e.runtime == nil {
		return false, errors.New("engine: agent runtime is required")
	}
	if e.checkpoints == nil {
		return false, errors.New("engine: checkpoint reader is required")
	}
	checkpoint, err := e.checkpoints.LoadCheckpoint(ctx, expected.RootProcessID)
	if err != nil {
		if isExecutorCheckpointLoss(err) {
			return false, nil
		}
		return false, fmt.Errorf("engine: load process tree: %w", err)
	}
	if err := checkpoint.ValidateFor(expected); err != nil {
		if isExecutorCheckpointLoss(err) {
			return false, nil
		}
		return false, fmt.Errorf("engine: validate executor checkpoint ownership: %w", err)
	}
	tree, err := decodeValidatedProcessTree(checkpoint)
	if err != nil {
		if isExecutorCheckpointLoss(err) {
			return false, nil
		}
		return false, fmt.Errorf("engine: decode process tree: %w", err)
	}
	if checkpoint.BuildID != e.buildID {
		return false, nil
	}
	if err := validateCheckpointUsage(tree, checkpoint.Usage); err != nil {
		return false, nil
	}
	if err := e.runtime.ValidateRestoreTree(tree); err != nil {
		if isExecutorCheckpointLoss(err) {
			return false, nil
		}
		return false, fmt.Errorf("engine: validate process tree: %w", err)
	}
	root, ok := tree.Root()
	if !ok {
		return false, nil
	}
	if err := agentruntime.ValidateResumableSnapshot(root); err != nil {
		return false, nil
	}
	return true, nil
}

func executorCheckpointLost(operation string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrExecutorCheckpointLost, operation, err)
}
