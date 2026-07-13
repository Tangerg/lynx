package sessions

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// Rollback truncates the chat history log to the boundary watermark and drops each run's
// durable record + dangling interrupt as ONE atomic write-set (§8.1), then cancels
// any in-process parked turns that were abandoned and purges the subagent child
// sessions spawned at/after the boundary. A keepMark < 0 (unknown watermark —
// chain terminal still in-flight / pre-watermark) leaves the log untouched
// rather than guessing at a boundary that was never recorded. An empty boundary
// (nothing dropped) is a no-op.
func (c *Coordinator) Rollback(ctx context.Context, sessionID string, boundary transcript.Boundary) error {
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
	// A dropped parked run held the session's durable admission slot; the write-set
	// terminalizes it (Terminate) so the session can start a fresh run afterward.
	// The partial unique index guarantees at most one non-terminal row per session.
	if err := c.s.ApplyRollback(ctx, RollbackPlan{
		SessionID:  sessionID,
		RunID:      parkedRunID(parked),
		KeepMark:   boundary.KeepMark,
		DropRunIDs: dropRunIDs,
		Terminate:  len(parked) > 0,
	}); err != nil {
		return err
	}
	for _, r := range parked {
		c.cancelTurn(ctx, r)
	}
	c.purgeChildrenAfter(ctx, sessionID, boundary.BoundaryTime)
	return nil
}

func parkedRunID(parked []RunTurnBinding) string {
	if len(parked) == 0 {
		return ""
	}
	return parked[0].RunID
}
