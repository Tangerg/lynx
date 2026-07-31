package agentexec

import (
	"context"
	"errors"
	"fmt"

	agentruntime "github.com/Tangerg/lynx/agent/runtime"
)

// ErrProcessSnapshotLost reports that a parked process cannot be reconstructed
// from its durable state and the owning application Run must be recovered lost.
var ErrProcessSnapshotLost = errors.New("agentexec: process snapshot lost")

// ResumableProcess reports whether processID has a compatible waiting
// continuation owned by this engine.
func (e *Engine) ResumableProcess(ctx context.Context, processID string) (bool, error) {
	if e == nil || e.runtime == nil {
		return false, errors.New("engine: agent runtime is required")
	}
	if e.processStore == nil {
		return false, errors.New("engine: ProcessStore is required")
	}
	state, checkpoint, err := e.processStore.LoadTree(ctx, processID)
	if err != nil {
		if isProcessSnapshotLoss(err) {
			return false, nil
		}
		return false, fmt.Errorf("engine: load process tree: %w", err)
	}
	tree, err := decodeProcessTreeState(state)
	if err != nil {
		if isProcessSnapshotLoss(err) {
			return false, nil
		}
		return false, fmt.Errorf("engine: decode process tree: %w", err)
	}
	if err := checkpoint.Validate(); err != nil {
		return false, nil
	}
	if checkpoint.BuildID != e.buildID {
		return false, nil
	}
	if err := validateCheckpointUsage(tree, checkpoint.Usage); err != nil {
		return false, nil
	}
	if err := e.runtime.ValidateRestoreTree(tree); err != nil {
		if isProcessSnapshotLoss(err) {
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

func processSnapshotLost(operation string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrProcessSnapshotLost, operation, err)
}
