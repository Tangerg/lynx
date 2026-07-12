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
// ([sessions.Coordinator.RollbackFiles]). This adapter owns only the wire: it
// decodes the intent, resolves the boundary by decoding the wire-shaped run
// blobs UNDER the coordinator's claimed slot (the [sessions.BoundaryResolver]),
// and shapes the response from the dropped runs.
func (s *Server) RollbackSession(ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	intent, err := rollbackIntentFromWire(in)
	if err != nil {
		return nil, err
	}

	var boundary transcript.Boundary
	var refByID map[string]protocol.RunRef
	var userByRun map[string][]protocol.ContentBlock
	resolve := func(ctx context.Context) (transcript.Boundary, error) {
		items, runs, err := s.rt.ListTranscript(ctx, in.SessionID)
		if err != nil {
			return transcript.Boundary{}, err
		}
		nodes, byID, err := runBoundaryNodes(runs)
		if err != nil {
			return transcript.Boundary{}, err
		}
		b, err := transcript.Timeline(nodes).BoundaryAt(in.ToRunID, true)
		if err != nil {
			return transcript.Boundary{}, wireBoundaryErr(err)
		}
		boundary, refByID, userByRun = b, byID, openingUserInputByRun(items)
		return b, nil
	}

	ses, err := s.sessions.RollbackFiles(ctx, s.coordinator, sessions.RollbackSpec{
		SessionID:      in.SessionID,
		ToRunID:        in.ToRunID,
		RestoreFiles:   intent.restoreFiles,
		RestoreHistory: intent.restoreHistory,
	}, resolve)
	if err != nil {
		return nil, wireRollbackErr(err, in.SessionID)
	}

	// Each dropped run reports its opening user input so the client can
	// re-populate the composer. Files-only rollback drops nothing from history.
	out := []protocol.DroppedRun{}
	if intent.restoreHistory {
		for _, rec := range boundary.Dropped {
			out = append(out, protocol.DroppedRun{Run: refByID[rec.ID], UserInput: userByRun[rec.ID]})
		}
	}
	sess := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &protocol.RollbackSessionResponse{Session: &sess, DroppedRuns: out}, nil
}

// RecoverRollbacks re-drives any file rollback a crash left unfinished (§8.5),
// re-restoring the working tree + re-applying the durable truncation for each
// logged intent. Called once at boot before the server serves, so no run
// contends. It supplies the coordinator a boundary lookup that decodes the
// wire-shaped run blobs — the wire knowledge that keeps the recovery loop in the
// application layer without the coordinator learning the protocol.
func (s *Server) RecoverRollbacks(ctx context.Context) error {
	return s.sessions.RecoverWorkspaceMutations(ctx, s.rollbackBoundary)
}

// rollbackBoundary rebuilds the durable rollback cut for a (sessionID, toRunID)
// from the durable run records — the same decode the live rollback path runs.
func (s *Server) rollbackBoundary(ctx context.Context, sessionID, toRunID string) (transcript.Boundary, error) {
	runs, err := s.rt.ListTranscriptRuns(ctx, sessionID)
	if err != nil {
		return transcript.Boundary{}, err
	}
	nodes, _, err := runBoundaryNodes(runs)
	if err != nil {
		return transcript.Boundary{}, err
	}
	return transcript.Timeline(nodes).BoundaryAt(toRunID, true)
}

// wireRollbackErr maps the rollback coordinator's sentinels onto their wire
// errors (the boundary sentinels are already wire-mapped inside the resolver).
func wireRollbackErr(err error, sessionID string) error {
	switch {
	case errors.Is(err, sessions.ErrSessionBusy):
		return fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, sessionID)
	case errors.Is(err, sessions.ErrCheckpointUnavailable):
		return protocol.ErrCheckpointUnavailable
	default:
		return wireSessionErr(err)
	}
}
