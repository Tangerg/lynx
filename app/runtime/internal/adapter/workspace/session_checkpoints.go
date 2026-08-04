package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

// SessionCheckpoints adapts workspace checkpoint operations to session
// lifecycle restoration and cleanup.
type SessionCheckpoints struct{ checkpoints *Checkpoints }

// NewSessionCheckpoints adapts workspace checkpoint operations to the session
// lifecycle's restore and cleanup port.
func NewSessionCheckpoints(checkpoints *Checkpoints) SessionCheckpoints {
	return SessionCheckpoints{checkpoints: checkpoints}
}

func (s SessionCheckpoints) Restore(ctx context.Context, sessionID, cwd, runID string) error {
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

func (s SessionCheckpoints) DropSession(sessionID string) error {
	return s.checkpoints.DropSession(sessionID)
}
