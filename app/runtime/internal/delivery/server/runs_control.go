package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// CancelRun hard-stops a running run (outcome:canceled, API.md §7.3).
// A parked run is also abandoned — its live parked turn is torn down
// and its open interrupt dropped so it stops surfacing as resumable.
func (s *Server) CancelRun(ctx context.Context, in protocol.CancelRunRequest) error {
	e, ok := s.runs.MarkCancel(in.RunID, in.Reason) // surfaced on the synthesized canceled outcome (S6)

	if !ok {
		if err := s.rt.CancelParkedRun(ctx, in.RunID); err != nil {
			if errors.Is(err, lifecycle.ErrRunNotFound) {
				return protocol.ErrRunNotFound
			}
			return err
		}
		return nil
	}

	if e.Payload != nil && e.Payload.cancel != nil {
		e.Payload.cancel()
	}
	if err := s.rt.CancelRunTurn(ctx, lifecycle.RunTurn{
		RunID:     in.RunID,
		SessionID: e.Record.SessionID,
		TurnID:    e.Record.TurnID,
	}); err != nil {
		return err
	}
	return nil
}

// SteerRun injects a user message into an actively-running run so the model
// reads it on its next tool round (runs.steer, API.md §6). Only an
// actively-pumping run is steerable — a parked run (waiting on an interrupt)
// is answered via runs.resume, and a finished one can't be steered — so a
// miss in the live run registry is run_not_found.
func (s *Server) SteerRun(ctx context.Context, in protocol.SteerRunRequest) error {
	e, ok := s.runs.Get(in.RunID)
	if !ok {
		return protocol.ErrRunNotFound
	}
	// The run can finish between the registry read and the inject — or its
	// steering queue can close as the turn terminates (the run is still in
	// s.runs while the pump drains). InjectSteering reports both as
	// ErrTurnNotFound; map it to the wire run_not_found symbol so the client
	// retries the message as a fresh send rather than seeing it silently dropped.
	if err := s.rt.InjectTurnSteering(ctx, turn.TurnHandle{SessionID: e.Record.SessionID, TurnID: e.Record.TurnID}, in.Message); err != nil {
		if errors.Is(err, turn.ErrTurnNotFound) {
			return protocol.ErrRunNotFound
		}
		return err
	}
	return nil
}
