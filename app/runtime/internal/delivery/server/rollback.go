package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// RollbackSession discards the runs after the kept boundary, truncating the
// session in place at a run granularity (AUX_API §4.1). Destructive: it
// truncates the conversation message log to the kept watermark, deletes the
// dropped runs' durable items + records, clears their dangling interrupts, and
// purges the subagent child sessions they spawned. ToRunID is inclusive-keep
// (omit = clear to empty). Rejected with session_busy while a run is in flight.
//
// The whole guarded operation — single-writer + working-tree admission, working
// tree restore, durable truncation — lives in the sessions coordinator
// ([sessions.Coordinator.Rollback]). This adapter only decodes the intent
// and presents the canonical result; boundary resolution stays in application.
func (s *Server) RollbackSession(ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	intent, err := rollbackIntentFromWire(in)
	if err != nil {
		return nil, err
	}

	result, err := s.sessions.Rollback(ctx, sessions.RollbackSpec{
		SessionID:      in.SessionID,
		ToRunID:        in.ToRunID,
		RestoreFiles:   intent.restoreFiles,
		RestoreHistory: intent.restoreHistory,
	})
	if err != nil {
		return nil, wireRollbackErr(err, in.SessionID)
	}

	// Each dropped run reports its opening user input so the client can
	// re-populate the composer. Files-only rollback drops nothing from history.
	out := []protocol.DroppedRun{}
	if intent.restoreHistory {
		for _, dropped := range result.Dropped {
			input := make([]protocol.ContentBlock, len(dropped.UserInput))
			for i, block := range dropped.UserInput {
				input[i] = presentContent(block)
			}
			out = append(out, protocol.DroppedRun{Run: presentRun(dropped.Run), UserInput: input})
		}
	}
	sess := sessionViewToWire(result.Session)
	return &protocol.RollbackSessionResponse{Session: &sess, DroppedRuns: out}, nil
}

// wireRollbackErr maps the rollback coordinator's sentinels onto their wire
// errors (the boundary sentinels are already wire-mapped inside the resolver).
func wireRollbackErr(err error, sessionID string) error {
	switch {
	case errors.Is(err, sessions.ErrSessionBusy):
		return fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, sessionID)
	case errors.Is(err, sessions.ErrCheckpointUnavailable):
		return protocol.ErrCheckpointUnavailable
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	default:
		return wireSessionErr(err)
	}
}

func wireBoundaryErr(err error) error {
	switch {
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	default:
		return err
	}
}
