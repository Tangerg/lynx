package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/lyra/internal/service/workspace"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// snapshotCheckpoint anchors the session's working tree at a finished run, so a
// later rollback{restoreType:files|both} can restore to it. Best-effort: a
// disabled store (git unavailable), an unresolvable cwd, or a git error never
// fails the run — the snapshot just doesn't exist, and a files restore that
// targets it returns checkpoint_unavailable.
func (s *Server) snapshotCheckpoint(ctx context.Context, sessionID, runID string) {
	cwd := s.sessionCwd(ctx, sessionID)
	if cwd == "" {
		return
	}
	_ = s.workspace.Snapshot(ctx, sessionID, cwd, runID)
}

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
