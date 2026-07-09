package lifecycle

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// RollbackBoundary is the resolved, domain-level rollback split.
type RollbackBoundary struct {
	KeepMark     int
	Dropped      []transcript.RunNode
	BoundaryTime time.Time
	DropRunIDs   []string
}

// ResolveRollbackBoundary computes the root-run inclusive-keep boundary for a
// rollback. The protocol adapter owns wire decoding; once it has lifted stored
// RunRefs into [transcript.RunNode], the decision is a lifecycle use-case, not
// protocol adaptation.
func ResolveRollbackBoundary(nodes []transcript.RunNode, toRunID string) (RollbackBoundary, error) {
	b, err := transcript.Timeline(nodes).BoundaryAt(toRunID, true)
	if err != nil {
		return RollbackBoundary{}, err
	}
	dropIDs := make([]string, len(b.Dropped))
	for i, rec := range b.Dropped {
		dropIDs[i] = rec.ID
	}
	return RollbackBoundary{
		KeepMark:     b.KeepMark,
		Dropped:      b.Dropped,
		BoundaryTime: b.BoundaryTime,
		DropRunIDs:   dropIDs,
	}, nil
}

// Rollback truncates the chat history log to keepMark and drops each run's
// durable items + record + dangling interrupt as ONE transaction, then cancels
// any in-process parked turns that were abandoned and purges the subagent child
// sessions spawned at/after purgeAfter. A keepMark < 0 (unknown watermark —
// chain terminal still in-flight / pre-watermark) leaves the log untouched
// rather than guessing at a boundary that was never recorded.
func (c *Coordinator) Rollback(ctx context.Context, turns TurnCanceler, sessionID string, keepMark int, dropRunIDs []string, purgeAfter time.Time) error {
	parked, err := c.parkedTurns(ctx, dropRunIDs)
	if err != nil {
		return err
	}
	if err := c.s.RunInTx(ctx, func(ctx context.Context) error {
		if keepMark >= 0 {
			if err := c.s.TruncateMessages(ctx, sessionID, keepMark); err != nil {
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
		c.cancelTurn(ctx, turns, r)
	}
	c.purgeChildrenAfter(ctx, sessionID, purgeAfter)
	return nil
}

// RollbackResolved executes a previously resolved rollback boundary.
func (c *Coordinator) RollbackResolved(ctx context.Context, turns TurnCanceler, sessionID string, b RollbackBoundary) error {
	if len(b.Dropped) == 0 {
		return nil
	}
	return c.Rollback(ctx, turns, sessionID, b.KeepMark, b.DropRunIDs, b.BoundaryTime)
}
