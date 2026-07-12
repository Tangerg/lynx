package bootstrap

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

// sessionCheckpoints adapts the workspace checkpoint store to the sessions
// coordinator's [sessions.WorkspaceCheckpoints] port: it restores a working tree
// to a run-boundary snapshot (mapping the adapter's own disabled/missing-snapshot
// sentinel onto [sessions.ErrCheckpointUnavailable] so the coordinator stays free
// of the adapter package) and drops a deleted session's snapshots.
type sessionCheckpoints struct{ cp *workspace.Checkpoints }

func (r sessionCheckpoints) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if err := r.cp.Restore(ctx, sessionID, cwd, runID); err != nil {
		if errors.Is(err, workspace.ErrCheckpointUnavailable) {
			return sessions.ErrCheckpointUnavailable
		}
		return err
	}
	return nil
}

func (r sessionCheckpoints) DropSession(sessionID string) error {
	return r.cp.DropSession(sessionID)
}
