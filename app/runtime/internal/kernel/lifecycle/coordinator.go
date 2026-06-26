// Package lifecycle owns the cross-domain atomic write-sets behind a few
// session/run lifecycle use-cases — rollback truncation, the session-delete
// cascade, the import/restore sequence, and the subagent subtree purge. Each
// spans several domain stores (the session row, the transcript, the chat-memory
// log, open interrupts) and several commit as ONE transaction via RunInTx, so a
// mid-sequence failure leaves no half-mutated session.
//
// These are use-case orchestration, not protocol adaptation: keeping them here
// (driven by the protocol adapter, which still owns wire decode, boundary
// decision, and busy guards) holds the "thin delivery" line and lets the
// write-sets be tested without standing up the wire. The adapter computes WHAT
// to mutate (e.g. the rollback boundary, decoded from wire blobs); the
// Coordinator EXECUTES the multi-domain mutation atomically.
package lifecycle

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Stores is the consumer-defined surface the Coordinator drives — the runtime's
// session-scoped stores plus the chat-memory log, the process-local resume
// gate (ForgetSession), and the transactional seam (RunInTx). The composition
// root's runtime bundle satisfies it; defined here so the Coordinator depends
// only on what it calls, not the whole runtime.
type Stores interface {
	Session() session.Service
	Transcript() transcript.Store
	Interrupts() interrupts.Store
	// TruncateMessages clamps a session's chat-memory log to keepN messages
	// (keepN=0 clears it).
	TruncateMessages(ctx context.Context, sessionID string, keepN int) error
	// SeedHistory replaces a session's chat-memory log with msgs (import).
	SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error
	// ForgetSession releases the turn service's process-local state for a
	// session that is being removed (the SessionStart gate) — see turn.Service.
	ForgetSession(sessionID string)
	// RunInTx runs fn inside one storage transaction; the store calls the
	// closure makes join it through the context.
	RunInTx(ctx context.Context, fn func(context.Context) error) error
}

// Coordinator executes session/run lifecycle write-sets across the domain
// stores. Stateless beyond its Stores handle; safe to share.
type Coordinator struct {
	s Stores
}

// New returns a Coordinator over s.
func New(s Stores) *Coordinator { return &Coordinator{s: s} }

// Rollback truncates the chat-memory log to keepMark and drops each run's
// durable items + record + dangling interrupt as ONE transaction, then purges
// the subagent child sessions spawned at/after purgeAfter. A keepMark < 0
// (unknown watermark — chain terminal still in-flight / pre-watermark) leaves
// the log untouched rather than guessing at a boundary that was never recorded.
// Rolling back over a parked run un-parks it (the interrupt delete).
//
// The caller (the protocol adapter) owns the wire-coupled decision — decoding
// the stored RunRefs, computing the boundary ([transcript.BoundaryAt]), and the
// session_busy guard — and passes only the resolved boundary here.
func (c *Coordinator) Rollback(ctx context.Context, sessionID string, keepMark int, dropRunIDs []string, purgeAfter time.Time) error {
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
	c.purgeChildrenAfter(ctx, sessionID, purgeAfter)
	return nil
}

// Fork creates a child of parentID, seeds it with the supplied chat-memory
// prefix, and renames it — as ONE transaction, so a mid-sequence failure rolls
// the whole fork back rather than leaving an orphaned child the user never saw
// succeed. The caller owns the wire-coupled half — decoding the stored RunRefs,
// computing the boundary ([transcript.BoundaryAt]), and reading + truncating the
// parent's history to that boundary — and passes only the resolved message
// prefix here. title == "" leaves the default "<parent> (fork)".
func (c *Coordinator) Fork(ctx context.Context, parentID string, msgs []chat.Message, title string) (session.Session, error) {
	var child session.Session
	if err := c.s.RunInTx(ctx, func(ctx context.Context) error {
		ch, err := c.s.Session().Fork(ctx, parentID, "")
		if err != nil {
			return err
		}
		if err := c.s.SeedHistory(ctx, ch.ID, msgs); err != nil {
			return err
		}
		if title != "" {
			if err := c.s.Session().Rename(ctx, ch.ID, title); err != nil {
				return err
			}
			ch.Title = title
		}
		child = ch
		return nil
	}); err != nil {
		return session.Session{}, err
	}
	return child, nil
}

// DeleteSession removes the session row (authoritative) then best-effort
// cascades the session-scoped storage: durable history, chat-memory, open
// interrupts, and the process-local resume gate. The cascade runs AFTER the
// authoritative delete so a partial cascade leaves harmless orphans, never a
// half-deleted session. File checkpoints (shadow git) are NOT dropped here —
// that is the adapter's workspace concern, not a storage write-set.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	if err := c.s.Session().Delete(ctx, sessionID); err != nil {
		return err
	}
	_ = c.s.Transcript().DeleteSession(ctx, sessionID) // history runs + items
	_ = c.s.TruncateMessages(ctx, sessionID, 0)        // chat-memory messages
	c.dropInterrupts(ctx, sessionID)                   // durable open interrupts
	c.s.ForgetSession(sessionID)                       // process-local SessionStart gate
	return nil
}

// RestoreSession recreates a session under its ORIGINAL id from a decoded
// artifact: it upserts the session record, replaces any existing history (drop
// old items/runs + clear the chat log), re-seeds the chat messages, and
// re-persists the runs + items — all in one transaction. Without the
// transaction a mid-sequence failure (after the destructive delete/truncate)
// would leave the session row live but its history half-destroyed.
//
// The caller decodes the wire artifact into these domain values; this method
// only commits them.
func (c *Coordinator) RestoreSession(ctx context.Context, ses session.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	return c.s.RunInTx(ctx, func(ctx context.Context) error {
		if err := c.s.Session().Restore(ctx, ses); err != nil {
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
// chat-memory messages, durable history (items + runs), the session row, and
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
	pending, err := c.s.Interrupts().List(ctx, sessionID)
	if err != nil {
		return
	}
	for _, p := range pending {
		_ = c.s.Interrupts().Delete(ctx, p.ParentRunID)
	}
}
