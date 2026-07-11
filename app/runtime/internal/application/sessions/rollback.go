package sessions

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// Rollback truncates the chat history log to the boundary watermark and drops each run's
// durable items + record + dangling interrupt as ONE transaction, then cancels
// any in-process parked turns that were abandoned and purges the subagent child
// sessions spawned at/after purgeAfter. A keepMark < 0 (unknown watermark —
// chain terminal still in-flight / pre-watermark) leaves the log untouched
// rather than guessing at a boundary that was never recorded. An empty boundary
// (nothing dropped) is a no-op.
func (c *Coordinator) Rollback(ctx context.Context, sessionID string, boundary transcript.Boundary) error {
	if len(boundary.Dropped) == 0 {
		return nil
	}
	dropRunIDs := boundary.DroppedRunIDs()
	parked, err := c.parkedTurns(ctx, dropRunIDs)
	if err != nil {
		return err
	}
	if err := c.s.RunInTx(ctx, func(ctx context.Context) error {
		if boundary.KeepMark >= 0 {
			if err := c.s.TruncateMessages(ctx, sessionID, boundary.KeepMark); err != nil {
				return err
			}
		}
		// Surfaced (not swallowed): after the truncate above commits, a failed
		// DeleteRun would otherwise leave a run record past the boundary whose
		// messages are already gone — an orphan inconsistent with the log.
		for _, runID := range dropRunIDs {
			if err := c.s.Transcript().DeleteRun(ctx, sessionID, runID); err != nil {
				return err
			}
			if err := c.s.Interrupts().Delete(ctx, runID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, r := range parked {
		c.cancelTurn(ctx, r)
	}
	// A dropped parked run held the session's durable admission slot; free it so
	// the session can start a fresh run after the rollback. The partial unique
	// index guarantees at most one non-terminal row per session, so a single
	// session-keyed terminalize covers whichever dropped run was parked.
	if len(parked) > 0 {
		c.terminalizeRun(ctx, sessionID)
	}
	c.purgeChildrenAfter(ctx, sessionID, boundary.BoundaryTime)
	return nil
}
