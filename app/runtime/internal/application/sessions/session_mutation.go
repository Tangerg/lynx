package sessions

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// DeleteSession atomically removes all durable session state (the atomic
// write-set), then tears down process-local parked turns and the resume gate,
// and finally drops the session's working-tree checkpoints. The open interrupts
// are read up front so the abandoned turns can be canceled after the durable
// state is gone. The checkpoint drop is best-effort cleanup (a shadow-git
// concern) run last, after the durable delete has already succeeded.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	pending, err := c.s.Interrupts().List(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := c.s.ApplyDelete(ctx, sessionID); err != nil {
		return err
	}
	for _, item := range pending {
		c.cancelTurn(ctx, RunTurnBinding{
			RunID:     item.RunID,
			SessionID: item.SessionID,
			TurnID:    item.TurnID,
		})
	}
	c.s.ForgetSession(sessionID)
	if c.checkpoints != nil {
		_ = c.checkpoints.DropSession(sessionID)
	}
	return nil
}

// RestoreSession recreates a session under its ORIGINAL id from a decoded
// artifact and replaces its whole history as one atomic write-set (§8.1): it
// upserts the session record, clears the old interrupts / admission rows /
// transcript / chat log, then re-seeds the messages, runs, and items. Without
// the single transaction a mid-sequence failure (after the destructive clear)
// would leave the session row live but its history half-destroyed.
//
// The caller decodes the wire artifact into these domain values. Restore still
// owns the Session admission boundary: it resolves Cwd exactly as Create and
// Update do before committing the decoded aggregate.
func (c *Coordinator) RestoreSession(ctx context.Context, ses session.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	cwd, err := c.resolveSessionCwd(ses.Cwd)
	if err != nil {
		return err
	}
	ses.Cwd = cwd
	return c.s.ApplyRestore(ctx, RestorePlan{
		Session:  ses,
		Messages: msgs,
		Runs:     runs,
		Items:    items,
	})
}

// PurgeSubtree deletes a session and its whole descendant subtree depth-first,
// each node removed by the atomic delete write-set. Best-effort — a partial
// failure still removes the leaves it reached (each node either deletes cleanly
// or is left for a later pass). Used to drop a failed-fork orphan and (via
// purgeChildrenAfter) the subagent children a rollback discards.
func (c *Coordinator) PurgeSubtree(ctx context.Context, sessionID string) {
	if children, err := c.s.Session().Children(ctx, sessionID); err == nil {
		for _, child := range children {
			c.PurgeSubtree(ctx, child.ID)
		}
	}
	_ = c.s.ApplyDelete(ctx, sessionID)
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
