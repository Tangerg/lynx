package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// CancelRun hard-stops a running run (outcome:canceled, API.md §7.3).
// A parked run is also abandoned — its live parked turn is torn down
// and its open interrupt dropped so it stops surfacing as resumable.
func (s *Server) CancelRun(ctx context.Context, in protocol.CancelRunRequest) error {
	err := s.coordinator.Cancel(ctx, runs.CancelCommand{RunID: in.RunID, Reason: in.Reason})
	switch {
	case errors.Is(err, runs.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, runs.ErrSessionBusy):
		return protocol.ErrSessionBusy
	default:
		return err
	}
}

// SteerRun injects a user message into the segment the request names so the model
// reads it on its next tool round (runs.steer, API.md §6).
//
// Only the addressed segment is steerable, and every other position says so by
// name: a waiting run is answered via runs.resume, a finished one cannot be
// steered at all, and a run that has moved to a different segment refuses rather
// than delivering the instruction to work the user never saw. The refusals are
// the same set a subscribe gets, because both are addressing one live segment.
func (s *Server) SteerRun(ctx context.Context, in protocol.SteerRunRequest) error {
	return wireLiveSegmentError(s.coordinator.Steer(ctx, runs.SteerCommand{
		RunID: in.RunID, ExpectedSegmentID: in.ExpectedSegmentID, Message: in.Message,
	}))
}
