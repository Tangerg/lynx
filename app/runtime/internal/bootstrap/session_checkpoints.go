package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

// sessionCheckpoints adapts the workspace checkpoint store to the sessions
// coordinator's [sessions.WorkspaceCheckpoints] port: it restores a working tree
// to a run-boundary snapshot (mapping the adapter's own disabled/missing-snapshot
// and possibly-incomplete-reset sentinels onto the application equivalents so
// the coordinator stays free of the adapter package) and drops a deleted
// session's snapshots.
type sessionCheckpoints struct{ cp *workspace.Checkpoints }

func (r sessionCheckpoints) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if err := r.cp.Restore(ctx, sessionID, cwd, runID); err != nil {
		switch {
		case errors.Is(err, workspace.ErrCheckpointUnavailable):
			return sessions.ErrCheckpointUnavailable
		case errors.Is(err, workspace.ErrCheckpointRestoreIncomplete):
			return fmt.Errorf("%w: %v", sessions.ErrCheckpointRestoreIncomplete, err)
		}
		return err
	}
	return nil
}

func (r sessionCheckpoints) DropSession(sessionID string) error {
	return r.cp.DropSession(sessionID)
}
