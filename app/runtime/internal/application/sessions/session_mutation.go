package sessions

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// DeleteSession atomically removes all durable session state, then tears down
// process-local parked turns and the resume gate. File checkpoints are an
// adapter-owned workspace concern and are dropped by the caller after commit.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	var pending []interrupts.Pending
	if err := c.s.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		pending, err = c.s.Interrupts().List(ctx, sessionID)
		if err != nil {
			return err
		}
		if err := c.s.Transcript().DeleteSession(ctx, sessionID); err != nil {
			return err
		}
		if err := c.s.TruncateMessages(ctx, sessionID, 0); err != nil {
			return err
		}
		for _, item := range pending {
			if err := c.s.Interrupts().Delete(ctx, item.ParentRunID); err != nil {
				return err
			}
		}
		if err := c.deleteRunRows(ctx, sessionID); err != nil {
			return err
		}
		return c.s.Session().Delete(ctx, sessionID)
	}); err != nil {
		return err
	}
	for _, item := range pending {
		c.cancelTurn(ctx, RunTurnBinding{
			RunID:     item.ParentRunID,
			SessionID: item.SessionID,
			TurnID:    item.TurnID,
		})
	}
	c.s.ForgetSession(sessionID)
	return nil
}

// RestoreSession recreates a session under its ORIGINAL id from a decoded
// artifact: it upserts the session record, clears old open interrupts, replaces
// any existing history (drop old items/runs + clear the chat log), re-seeds the
// chat messages, and re-persists the runs + items — all in one transaction.
// Without the transaction a mid-sequence failure (after the destructive
// delete/truncate) would leave the session row live but its history
// half-destroyed.
//
// The caller decodes the wire artifact into these domain values; this method
// only commits them.
func (c *Coordinator) RestoreSession(ctx context.Context, ses session.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	return c.s.RunInTx(ctx, func(ctx context.Context) error {
		if err := c.s.Session().Restore(ctx, ses); err != nil {
			return err
		}
		if err := c.deleteInterrupts(ctx, ses.ID); err != nil {
			return err
		}
		if err := c.deleteRunRows(ctx, ses.ID); err != nil {
			return err
		}
		if err := c.s.Transcript().DeleteSession(ctx, ses.ID); err != nil {
			return err
		}
		if err := c.s.TruncateMessages(ctx, ses.ID, 0); err != nil {
			return err
		}
		if err := c.s.SeedHistory(ctx, ses.ID, msgs); err != nil {
			return err
		}
		for _, r := range runs {
			if err := c.s.Transcript().PutRun(ctx, r); err != nil {
				return err
			}
		}
		for _, it := range items {
			if err := c.s.Transcript().AppendItem(ctx, it); err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeSubtree deletes a session and its whole descendant subtree depth-first:
// chat history messages, durable history (items + runs), the session row, and
// the process-local resume gate. Best-effort — a partial failure still removes
// the leaves it reached. Used to drop a failed-fork orphan and (via
// purgeChildrenAfter) the subagent children a rollback discards.
func (c *Coordinator) PurgeSubtree(ctx context.Context, sessionID string) {
	if children, err := c.s.Session().Children(ctx, sessionID); err == nil {
		for _, child := range children {
			c.PurgeSubtree(ctx, child.ID)
		}
	}
	_ = c.s.TruncateMessages(ctx, sessionID, 0)
	_ = c.s.Transcript().DeleteSession(ctx, sessionID)
	c.dropInterrupts(ctx, sessionID)
	_ = c.deleteRunRows(ctx, sessionID)
	_ = c.s.Session().Delete(ctx, sessionID)
	c.s.ForgetSession(sessionID)
}

// purgeChildrenAfter purges the subagent child sessions of parentID spawned
// at/after boundary (a zero boundary purges all children — the drop-all
// rollback). Attribution is by spawn time: a subtask of a kept run started
// before the boundary, one of a dropped run at/after it. Exact because a
// session runs its turns sequentially and rollback requires it idle, so run
// windows don't overlap.
func (c *Coordinator) purgeChildrenAfter(ctx context.Context, parentID string, boundary time.Time) {
	children, err := c.s.Session().Children(ctx, parentID)
	if err != nil {
		return
	}
	for _, child := range children {
		if !boundary.IsZero() && child.StartedAt.Before(boundary) {
			continue
		}
		c.PurgeSubtree(ctx, child.ID)
	}
}

// dropInterrupts removes every open-interrupt record for a session. Best-effort:
// a failed list or delete leaves a resumable record that a later pass can clear.
func (c *Coordinator) dropInterrupts(ctx context.Context, sessionID string) {
	_ = c.deleteInterrupts(ctx, sessionID)
}

func (c *Coordinator) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := c.s.Interrupts().List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := c.s.Interrupts().Delete(ctx, p.ParentRunID); err != nil {
			return err
		}
	}
	return nil
}
