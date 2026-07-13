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

// subtaskSessionsAfter resolves the internal subtask subtrees a rollback must
// delete. User-created forks share ParentID but have an empty Kind and remain
// independent conversations; only KindSubtask roots are attributed to runs.
// IDs are post-order so descendants are deleted before their parent.
func (c *Coordinator) subtaskSessionsAfter(ctx context.Context, parentID string, boundary time.Time) ([]string, error) {
	children, err := c.s.Session().Children(ctx, parentID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, child := range children {
		if child.Kind != session.KindSubtask {
			continue
		}
		if !boundary.IsZero() && child.StartedAt.Before(boundary) {
			continue
		}
		if err := c.appendSessionSubtree(ctx, child.ID, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c *Coordinator) appendSessionSubtree(ctx context.Context, sessionID string, out *[]string) error {
	children, err := c.s.Session().Children(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := c.appendSessionSubtree(ctx, child.ID, out); err != nil {
			return err
		}
	}
	*out = append(*out, sessionID)
	return nil
}
