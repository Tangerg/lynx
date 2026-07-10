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
	e, ok := s.runs.Get(in.RunID)

	if !ok {
		// Parked cancel and resume claim the same session admission slot. This
		// prevents a failed rehydrate's compensating Put from racing a cancel's
		// Delete and resurrecting an interrupt the user just abandoned.
		pending, admission, err := s.rt.ClaimResumeSlot(ctx, sessionClaimer{s: s}, in.RunID)
		if err != nil {
			switch {
			case errors.Is(err, lifecycle.ErrInterruptNotOpen):
				return protocol.ErrRunNotFound
			case errors.Is(err, lifecycle.ErrSessionBusy):
				return protocol.ErrSessionBusy
			default:
				return err
			}
		}
		defer admission.Release()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
		defer cancel()
		return s.rt.CancelRunBinding(cleanupCtx, lifecycle.RunTurnBinding{
			RunID:     in.RunID,
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
		})
	}

	// Mark the delivery handle before touching the durable binding. The handle's
	// lock is also held by interrupt commit + publication, so CancelRun cannot
	// delete an interrupt immediately before the pump recreates it.
	if e.Payload != nil {
		e.Payload.requestCancel(in.Reason)
	}
	// Keep the domain snapshot useful to concurrent readers. The pump reads the
	// reason from the handle because it may synthesize canceled before this
	// registry update runs.
	_, _ = s.runs.MarkCancel(in.RunID, in.Reason)
	cleanupCtx, cancel := e.Payload.cleanupContext(ctx)
	defer cancel()
	if err := s.rt.CancelRunBinding(cleanupCtx, lifecycle.RunTurnBinding{
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
