package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// applyRollback truncates the chat history log to the boundary watermark and drops each run's
// durable record + dangling interrupt as ONE atomic write-set (§8.1), then cancels
// any in-process parked turns that were abandoned and purges the subagent child
// sessions spawned at/after the boundary. A keepMark < 0 (unknown watermark —
// chain terminal still in-flight / pre-watermark) leaves the log untouched
// rather than guessing at a boundary that was never recorded. An empty boundary
// (nothing dropped) is a no-op. The caller resolves and claims dropSessionIDs
// before any cross-resource file restore starts.
func (c *Coordinator) applyRollback(ctx context.Context, sessionID string, boundary transcript.Boundary, dropSessionIDs []string) error {
	if len(boundary.Dropped) == 0 {
		return nil
	}
	dropRunIDs := boundary.DroppedRunIDs()
	// Read the parked turns BEFORE the write-set consumes their interrupts — the
	// in-process turns still need canceling once the durable records are gone.
	parked, err := c.parkedTurns(ctx, dropRunIDs)
	if err != nil {
		return err
	}
	childParked, err := c.parkedSessionTurns(ctx, dropSessionIDs)
	if err != nil {
		return err
	}
	// A dropped parked run held the session's durable admission slot; the write-set
	// terminalizes it (Terminate) so the session can start a fresh run afterward.
	// The partial unique index guarantees at most one non-terminal row per session.
	sessionIDs := append([]string{sessionID}, dropSessionIDs...)
	if err := c.withGoalMutation(ctx, sessionIDs, func(ctx context.Context) error {
		if c.writes == nil {
			return errors.New("sessions: write sets are unavailable")
		}
		return c.writes.ApplyRollback(ctx, RollbackPlan{
			SessionID:      sessionID,
			RunID:          parkedRunID(parked),
			KeepMark:       boundary.KeepMark,
			DropRunIDs:     dropRunIDs,
			DropSessionIDs: dropSessionIDs,
			ProcessIDs:     parkedProcessIDs(parked),
			Terminate:      len(parked) > 0,
		})
	}); err != nil {
		return err
	}
	var cleanupErrs []error
	for _, r := range slices.Concat(parked, childParked) {
		if err := c.cancelTurn(ctx, r); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	for _, id := range dropSessionIDs {
		if c.forgetter != nil {
			c.forgetter.ForgetSession(id)
		}
		if c.checkpoints != nil {
			if err := c.checkpoints.DropSession(id); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("sessions: drop checkpoints for rolled-back subtask session %q: %w", id, err))
			}
		}
		if c.sandbox != nil {
			if err := c.sandbox.Discard(id); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("sessions: discard sandbox copy for rolled-back subtask session %q: %w", id, err))
			}
		}
	}
	return errors.Join(cleanupErrs...)
}

func parkedProcessIDs(parked []RunTurnBinding) []string {
	ids := make([]string, 0, len(parked))
	for _, binding := range parked {
		if binding.ProcessID != "" {
			ids = append(ids, binding.ProcessID)
		}
	}
	return ids
}

func parkedRunID(parked []RunTurnBinding) string {
	if len(parked) == 0 {
		return ""
	}
	return parked[0].RunID
}

func (c *Coordinator) parkedSessionTurns(ctx context.Context, sessionIDs []string) ([]RunTurnBinding, error) {
	var out []RunTurnBinding
	for _, sessionID := range sessionIDs {
		if c.interrupts == nil {
			return nil, errors.New("sessions: interrupt store is unavailable")
		}
		pending, err := c.interrupts.List(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		for _, item := range pending {
			out = append(out, RunTurnBinding{RunID: item.RunID, SessionID: item.SessionID, TurnID: item.TurnID, ProcessID: item.ProcessID})
		}
	}
	return out, nil
}
