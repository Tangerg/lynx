package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// applyRollback truncates the chat history log to the boundary watermark and drops each run's
// durable record + dangling interrupt as ONE atomic write-set (§8.1), then cancels
// any in-process parked turns that were abandoned. Delegated work is represented
// by first-class child Runs in this same session, so there is no parallel hidden
// Session tree to infer or purge. A keepMark < 0 (unknown watermark —
// chain terminal still in-flight / pre-watermark) leaves the log untouched
// rather than guessing at a boundary that was never recorded. An empty boundary
// (nothing dropped) is a no-op.
func (c *Coordinator) applyRollback(ctx context.Context, sessionID string, boundary transcript.Boundary) error {
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
	// Read the boundary's task list BEFORE the write-set drops the runs after it: the
	// kept run survives, but reading inside the plan would mean the adapter deciding
	// which boundary the state comes from.
	todos, err := c.todoBoundary(ctx, boundary.KeepRunID)
	if err != nil {
		return err
	}
	// A dropped parked run held the session's durable admission slot; dropping its
	// record releases the slot, so the session can start a fresh run afterward.
	return c.withGoalMutation(
		ctx,
		[]string{sessionID},
		func(ctx context.Context) error {
			if c.writes == nil {
				return errors.New("sessions: write sets are unavailable")
			}
			return c.writes.ApplyRollback(ctx, RollbackPlan{
				SessionID:         sessionID,
				KeepMark:          boundary.KeepMark,
				DropRunIDs:        dropRunIDs,
				CheckpointRootIDs: parkedCheckpointRootIDs(parked),
				Todos:             todos,
			})
		},
		func(ctx context.Context) error {
			// The truncation is committed: the dropped Run subtree is gone and the
			// boundary's task list is published. Delegated work has no parallel
			// Session identity to clean up.
			c.publishAggregateMoved([]string{sessionID}, dropRunIDs)
			var cleanupErrs []error
			for _, r := range parked {
				if err := c.cancelTurn(ctx, r); err != nil {
					cleanupErrs = append(cleanupErrs, err)
				}
			}
			return errors.Join(cleanupErrs...)
		},
	)
}

func parkedCheckpointRootIDs(parked []RunTurnBinding) []string {
	ids := make([]string, 0, len(parked))
	for _, binding := range parked {
		if binding.CheckpointRootID != "" {
			ids = append(ids, binding.CheckpointRootID)
		}
	}
	return ids
}
