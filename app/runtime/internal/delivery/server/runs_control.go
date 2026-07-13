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

// SteerRun injects a user message into an actively-running run so the model
// reads it on its next tool round (runs.steer, API.md §6). Only an
// actively-pumping run is steerable — a parked run (waiting on an interrupt)
// is answered via runs.resume, and a finished one can't be steered — so a
// miss in the live run registry is run_not_found.
func (s *Server) SteerRun(ctx context.Context, in protocol.SteerRunRequest) error {
	err := s.coordinator.Steer(ctx, runs.SteerCommand{RunID: in.RunID, Message: in.Message})
	if errors.Is(err, runs.ErrRunNotFound) {
		return protocol.ErrRunNotFound
	}
	return err
}
