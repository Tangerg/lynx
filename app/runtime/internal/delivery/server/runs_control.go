package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// CancelRun hard-stops a running run (outcome:canceled, API.md §7.3).
// A parked run is also abandoned — its live parked turn is torn down
// and its open interrupt dropped so it stops surfacing as resumable.
func (s *Server) CancelRun(ctx context.Context, in protocol.CancelRunRequest) error {
	binding, cleanupCtx, cancel, ok := s.coordinator.BeginCancel(ctx, in.RunID, in.Reason)
	if !ok {
		// Not live — the parked-cancel path. Parked cancel and resume claim the
		// same session admission slot, so a failed rehydrate's compensating Put
		// can't race a cancel's Delete and resurrect an abandoned interrupt.
		pending, admission, err := s.sessions.ClaimResumeSlot(ctx, s.coordinator, in.RunID)
		if err != nil {
			switch {
			case errors.Is(err, sessions.ErrInterruptNotOpen):
				return protocol.ErrRunNotFound
			case errors.Is(err, sessions.ErrSessionBusy):
				return protocol.ErrSessionBusy
			default:
				return err
			}
		}
		defer admission.Release()
		pcleanupCtx, pcancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
		defer pcancel()
		return s.sessions.CancelRunBinding(pcleanupCtx, sessions.RunTurnBinding{
			RunID:     in.RunID,
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
		})
	}
	// Live: BeginCancel already marked the handle (before the durable binding, so
	// a cancel can't delete an interrupt the pump is about to recreate) and gave
	// us a cleanup context rooted on the run's owner (it survives request cancel).
	defer cancel()
	return s.sessions.CancelRunBinding(cleanupCtx, sessions.RunTurnBinding{
		RunID:     in.RunID,
		SessionID: binding.SessionID,
		TurnID:    binding.TurnID,
	})
}

// SteerRun injects a user message into an actively-running run so the model
// reads it on its next tool round (runs.steer, API.md §6). Only an
// actively-pumping run is steerable — a parked run (waiting on an interrupt)
// is answered via runs.resume, and a finished one can't be steered — so a
// miss in the live run registry is run_not_found.
func (s *Server) SteerRun(ctx context.Context, in protocol.SteerRunRequest) error {
	rec, ok := s.coordinator.LiveRun(in.RunID)
	if !ok {
		return protocol.ErrRunNotFound
	}
	// The run can finish between the registry read and the inject — or its
	// steering queue can close as the turn terminates (the run is still tracked
	// while the pump drains). InjectSteering reports both as ErrTurnNotFound; map
	// it to the wire run_not_found symbol so the client retries the message as a
	// fresh send rather than seeing it silently dropped.
	if err := s.rt.InjectTurnSteering(ctx, turn.TurnHandle{SessionID: rec.SessionID, TurnID: rec.TurnID}, in.Message); err != nil {
		if errors.Is(err, turn.ErrTurnNotFound) {
			return protocol.ErrRunNotFound
		}
		return err
	}
	return nil
}
