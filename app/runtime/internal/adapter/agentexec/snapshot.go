package agentexec

import (
	"context"
	"errors"
	"fmt"
)

// ErrProcessSnapshotLost reports that a parked process cannot be reconstructed
// from its durable state and the owning application Run must be recovered lost.
var ErrProcessSnapshotLost = errors.New("agentexec: process snapshot lost")

// ResumableProcess reports whether processID has a compatible waiting
// continuation owned by this engine.
func (e *Engine) ResumableProcess(ctx context.Context, processID string) (bool, error) {
	if e == nil || e.turnRestorer == nil {
		return false, errors.New("engine: process restorer is required")
	}
	return e.turnRestorer.Resumable(ctx, processID)
}

func processSnapshotLost(operation string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrProcessSnapshotLost, operation, err)
}
