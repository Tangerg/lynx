package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// restoreCheckpoint resets the session's working tree to the runID snapshot,
// mapping a missing snapshot / disabled store onto the wire
// checkpoint_unavailable so the caller can keep history untouched (atomic
// "both": files first).
func (s *Server) restoreCheckpoint(ctx context.Context, sessionID, runID string) error {
	cwd := s.sessionCwd(ctx, sessionID)
	if cwd == "" {
		return protocol.ErrCheckpointUnavailable
	}
	if err := s.workspace.Restore(ctx, sessionID, cwd, runID); err != nil {
		if errors.Is(err, workspace.ErrCheckpointUnavailable) {
			return protocol.ErrCheckpointUnavailable
		}
		return err
	}
	return nil
}

// dropCheckpoints removes a session's shadow repo (on session delete).
func (s *Server) dropCheckpoints(sessionID string) {
	_ = s.workspace.DropSession(sessionID)
}
