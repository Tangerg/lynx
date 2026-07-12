package bootstrap

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

// checkpointRestorer adapts the workspace checkpoint store to the sessions
// coordinator's [sessions.WorkspaceRestorer] port, mapping the adapter's own
// disabled/missing-snapshot sentinel onto [sessions.ErrCheckpointUnavailable] so
// the coordinator stays free of the adapter package.
type checkpointRestorer struct{ cp *workspace.Checkpoints }

func (r checkpointRestorer) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if err := r.cp.Restore(ctx, sessionID, cwd, runID); err != nil {
		if errors.Is(err, workspace.ErrCheckpointUnavailable) {
			return sessions.ErrCheckpointUnavailable
		}
		return err
	}
	return nil
}
