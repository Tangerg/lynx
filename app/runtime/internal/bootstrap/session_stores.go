package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// sessionForgetter releases the turn dispatcher's process-local state for a
// session being removed (the SessionStart gate). The kernel turn dispatcher
// satisfies it; the sessions coordinator's Stores surface calls it after a
// delete/purge cascade commits.
type sessionForgetter interface {
	ForgetSession(sessionID string)
}

// sessionStores is the composition root's adapter from the assembled durable
// stores to the sessions coordinator's [sessions.Stores] surface. Besides the
// session-scoped read stores and the resume gate, it OWNS the atomic durable
// write-sets ([sessions.WriteSets]): each applies its whole multi-store mutation
// in one transaction here, so the coordinator commits an atomic decision (roll
// back / restore / delete / cancel) rather than stitching a transaction across
// table-CRUD calls (§8.1/§8.4).
type sessionStores struct {
	sessions   *sqlitestore.SessionStore
	transcript *sqlitestore.TranscriptStore
	interrupts *sqlitestore.InterruptStore
	runs       *sqlitestore.RunStateStore
	history    *conversation.Messages
	forgetter  sessionForgetter
	tx         lyraruntime.Transactor
}

func (s sessionStores) Session() sessions.SessionStore      { return s.sessions }
func (s sessionStores) Interrupts() sessions.InterruptStore { return s.interrupts }

func (s sessionStores) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return s.history.Read(ctx, sessionID)
}

func (s sessionStores) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return s.history.Seed(ctx, sessionID, msgs)
}

func (s sessionStores) ForgetSession(sessionID string) { s.forgetter.ForgetSession(sessionID) }

// RunInTx runs fn inside one storage transaction, falling back to a direct call
// when no transactor is wired (a non-sqlite / test runtime) — see
// [lyraruntime.Transactor]. Used by the fork/patch aggregate write-sets that stay
// single-domain; the destructive multi-store write-sets go through the Apply*
// methods below.
func (s sessionStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	if s.tx == nil {
		return fn(ctx)
	}
	return s.tx(ctx, fn)
}

// ApplyRollback truncates the chat log to the boundary watermark, drops each
// past-boundary run's transcript record + open interrupt, and terminalizes an
// abandoned parked run's admission row — all in one transaction (§8.1/§8.3).
func (s sessionStores) ApplyRollback(ctx context.Context, plan execution.RollbackPlan) error {
	return s.RunInTx(ctx, func(ctx context.Context) error {
		if plan.KeepMark >= 0 {
			if err := s.history.Truncate(ctx, plan.SessionID, plan.KeepMark); err != nil {
				return err
			}
		}
		// Surfaced (not swallowed): after the truncate commits, a failed DeleteRun
		// would leave a run record past the boundary whose messages are already
		// gone — an orphan inconsistent with the log.
		for _, runID := range plan.DropRunIDs {
			if err := s.transcript.DeleteRun(ctx, plan.SessionID, runID); err != nil {
				return err
			}
			if err := s.interrupts.Delete(ctx, runID); err != nil {
				return err
			}
		}
		if plan.Terminate {
			return s.runs.Terminalize(ctx, plan.SessionID, execution.OutcomeCanceled.String())
		}
		return nil
	})
}

// ApplyRestore recreates a session under its original id and replaces its whole
// history atomically: clear the old interrupts / admission rows / transcript /
// chat log, then seed the decoded messages, runs, and items.
func (s sessionStores) ApplyRestore(ctx context.Context, plan execution.RestorePlan) error {
	id := plan.Session.ID
	return s.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.sessions.Restore(ctx, plan.Session); err != nil {
			return err
		}
		if err := s.deleteInterrupts(ctx, id); err != nil {
			return err
		}
		if err := s.runs.DeleteForSession(ctx, id); err != nil {
			return err
		}
		if err := s.transcript.DeleteSession(ctx, id); err != nil {
			return err
		}
		if err := s.history.Truncate(ctx, id, 0); err != nil {
			return err
		}
		if err := s.history.Seed(ctx, id, plan.Messages); err != nil {
			return err
		}
		for _, r := range plan.Runs {
			if err := s.transcript.PutRun(ctx, r); err != nil {
				return err
			}
		}
		for _, it := range plan.Items {
			if err := s.transcript.AppendItem(ctx, it); err != nil {
				return err
			}
		}
		return nil
	})
}

// ApplyDelete removes all of a session's durable state — transcript, chat log,
// open interrupts, admission rows, and the session row — atomically.
func (s sessionStores) ApplyDelete(ctx context.Context, sessionID string) error {
	return s.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.transcript.DeleteSession(ctx, sessionID); err != nil {
			return err
		}
		if err := s.history.Truncate(ctx, sessionID, 0); err != nil {
			return err
		}
		if err := s.deleteInterrupts(ctx, sessionID); err != nil {
			return err
		}
		if err := s.runs.DeleteForSession(ctx, sessionID); err != nil {
			return err
		}
		return s.sessions.Delete(ctx, sessionID)
	})
}

// ApplyCancel abandons a parked run: it drops the open interrupt and terminalizes
// the run's admission row atomically, so a canceled parked run neither stays
// resumable nor leaves the session durably busy.
func (s sessionStores) ApplyCancel(ctx context.Context, sessionID, runID string) error {
	return s.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.interrupts.Delete(ctx, runID); err != nil {
			return err
		}
		return s.runs.Terminalize(ctx, sessionID, execution.OutcomeCanceled.String())
	})
}

// deleteInterrupts removes every open-interrupt record for a session inside the
// caller's transaction — the list + per-row delete join the same conn(ctx).
func (s sessionStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := s.interrupts.Delete(ctx, p.ParentRunID); err != nil {
			return err
		}
	}
	return nil
}
