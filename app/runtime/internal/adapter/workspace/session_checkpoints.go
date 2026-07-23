package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

type sessionCheckpoints struct{ checkpoints *Checkpoints }

// NewSessionCheckpoints adapts workspace checkpoint operations to the session
// lifecycle's restore and cleanup port.
func NewSessionCheckpoints(checkpoints *Checkpoints) sessions.WorkspaceCheckpoints {
	return sessionCheckpoints{checkpoints: checkpoints}
}

func (s sessionCheckpoints) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if err := s.checkpoints.Restore(ctx, sessionID, cwd, runID); err != nil {
		switch {
		case errors.Is(err, ErrCheckpointUnavailable):
			return sessions.ErrCheckpointUnavailable
		case errors.Is(err, ErrCheckpointRestoreIncomplete):
			return fmt.Errorf("%w: %v", sessions.ErrCheckpointRestoreIncomplete, err)
		default:
			return err
		}
	}
	return nil
}

func (s sessionCheckpoints) DropSession(sessionID string) error {
	return s.checkpoints.DropSession(sessionID)
}
