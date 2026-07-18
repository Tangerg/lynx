package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// sessionForgetter releases the turn dispatcher's process-local state for a
// session being removed (the SessionStart gate). The turn dispatcher
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
	sessions    *sqlitestore.SessionStore
	transcript  *sqlitestore.TranscriptStore
	interrupts  *sqlitestore.InterruptStore
	runs        *sqlitestore.RunStateStore
	processes   *sqlitestore.ProcessStore
	history     *conversation.Messages
	todos       todo.Store
	approvals   approval.RuleStore
	toolResults *sqlitestore.ToolResultStore
	forgetter   sessionForgetter
	tx          Transactor
}

func (s sessionStores) Session() sessions.SessionStore       { return s.sessions }
func (s sessionStores) Interrupts() sessions.InterruptStore  { return s.interrupts }
func (s sessionStores) Transcript() sessions.TranscriptStore { return s.transcript }

func (s sessionStores) ReadSnapshot(ctx context.Context, sessionID string) (sessions.Snapshot, error) {
	var snapshot sessions.Snapshot
	err := s.runInTx(ctx, func(ctx context.Context) error {
		ses, err := s.sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		messages, err := s.history.Read(ctx, sessionID)
		if err != nil {
			return err
		}
		items, runs, err := s.transcript.List(ctx, sessionID)
		if err != nil {
			return err
		}
		var toolResults []offload.ToolResultBlob
		if s.toolResults != nil {
			toolResults, err = s.toolResults.List(ctx, sessionID)
			if err != nil {
				return err
			}
		}
		snapshot = sessions.Snapshot{
			Session: ses, Messages: messages, Items: items, Runs: runs, ToolResults: toolResults,
		}
		return snapshot.ValidateToolResults()
	})
	return snapshot, err
}

func (s sessionStores) ForgetSession(sessionID string) { s.forgetter.ForgetSession(sessionID) }

// runInTx runs fn inside the required storage transaction. The Apply* write-sets
// below drive it; it is the one transactional seam left, behind atomic ports
// rather than the coordinator's own surface (§8.4).
func (s sessionStores) runInTx(ctx context.Context, fn func(context.Context) error) error {
	return s.tx(ctx, fn)
}

// ApplyFork branches a child session off plan.ParentID, seeds its chat log with
// the resolved history prefix, and titles it — all in one transaction, so a
// concurrent delete on the parent can't race a half-created child.
func (s sessionStores) ApplyFork(ctx context.Context, plan sessions.ForkPlan) (session.Session, error) {
	var child session.Session
	err := s.runInTx(ctx, func(ctx context.Context) error {
		ch, err := s.sessions.Fork(ctx, plan.ParentID, "")
		if err != nil {
			return err
		}
		if err := s.history.Seed(ctx, ch.ID, plan.Messages); err != nil {
			return err
		}
		if plan.Title != "" {
			if err := s.sessions.Rename(ctx, ch.ID, plan.Title); err != nil {
				return err
			}
			ch.Title = plan.Title
		}
		child = ch
		return nil
	})
	if err != nil {
		return session.Session{}, err
	}
	return child, nil
}

// ApplyRollback truncates the chat log to the boundary watermark, drops each
// past-boundary run, terminalizes an abandoned parked run, and deletes the
// attributed internal subtask subtrees and their process snapshots — all in one
// transaction (§8.1/§8.3).
func (s sessionStores) ApplyRollback(ctx context.Context, plan sessions.RollbackPlan) error {
	return s.runInTx(ctx, func(ctx context.Context) error {
		// Todos describe the current future plan, not historical transcript state.
		// A rewind invalidates that projection, so clear it in the same commit.
		if s.todos != nil {
			if err := s.todos.DeleteSession(ctx, plan.SessionID); err != nil {
				return err
			}
		}
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
		for _, processID := range plan.ProcessIDs {
			if err := s.processes.DeleteTree(ctx, processID); err != nil {
				return err
			}
		}
		if plan.Terminate {
			if err := s.runs.Terminalize(ctx, plan.SessionID, plan.RunID, execution.OutcomeCanceled); err != nil {
				return err
			}
		}
		for _, sessionID := range plan.DropSessionIDs {
			if err := s.deleteSession(ctx, sessionID); err != nil {
				return err
			}
		}
		return nil
	})
}

// ApplyRestore recreates a session under its original id and replaces its whole
// history atomically: clear the old interrupts / admission rows / transcript /
// chat log, then seed the decoded messages, runs, and items.
func (s sessionStores) ApplyRestore(ctx context.Context, plan sessions.RestorePlan) error {
	normalized, err := plan.NormalizeForRestore()
	if err != nil {
		return err
	}
	plan = normalized
	id := plan.Session.ID
	return s.runInTx(ctx, func(ctx context.Context) error {
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
		if s.toolResults != nil {
			if err := s.toolResults.DropSession(ctx, id); err != nil {
				return err
			}
		} else if len(plan.ToolResults) > 0 {
			return errors.New("bootstrap: cannot restore tool results without blob persistence")
		}
		if err := s.history.Truncate(ctx, id, 0); err != nil {
			return err
		}
		if s.todos != nil {
			if err := s.todos.DeleteSession(ctx, id); err != nil {
				return err
			}
		}
		if s.approvals != nil {
			if err := s.approvals.DeleteSession(ctx, id); err != nil {
				return err
			}
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
		for _, blob := range plan.ToolResults {
			if err := s.toolResults.Restore(ctx, blob); err != nil {
				return err
			}
		}
		return nil
	})
}

// ApplyDelete removes all of a session's durable state — transcript, chat log,
// open interrupts, process snapshots, admission rows, and the session row —
// atomically.
func (s sessionStores) ApplyDelete(ctx context.Context, plan sessions.DeletePlan) error {
	if len(plan.SessionIDs) == 0 {
		return errors.New("runtime: delete plan has no sessions")
	}
	return s.runInTx(ctx, func(ctx context.Context) error {
		for _, sessionID := range plan.SessionIDs {
			if err := s.deleteSession(ctx, sessionID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s sessionStores) deleteSession(ctx context.Context, sessionID string) error {
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
	if s.todos != nil {
		if err := s.todos.DeleteSession(ctx, sessionID); err != nil {
			return err
		}
	}
	if s.approvals != nil {
		if err := s.approvals.DeleteSession(ctx, sessionID); err != nil {
			return err
		}
	}
	if s.toolResults != nil {
		// Offloaded tool-result bodies are session-scoped; drop them in the same
		// cascade transaction so a deleted session leaves no orphan blobs.
		if err := s.toolResults.DropSession(ctx, sessionID); err != nil {
			return err
		}
	}
	return s.sessions.Delete(ctx, sessionID)
}

// ApplyTerminal ends a parked run: it drops the open interrupt + process
// snapshot and closes the run's admission row atomically, so the abandoned run
// neither stays resumable nor leaves the session durably busy.
func (s sessionStores) ApplyTerminal(ctx context.Context, plan sessions.TerminalPlan) error {
	return s.runInTx(ctx, func(ctx context.Context) error {
		for _, item := range plan.Items {
			if err := s.transcript.AppendItem(ctx, item); err != nil {
				return err
			}
		}
		if err := s.transcript.PutRun(ctx, plan.Run); err != nil {
			return err
		}
		if plan.ProcessID != "" {
			if err := s.processes.DeleteTree(ctx, plan.ProcessID); err != nil {
				return err
			}
		}
		if err := s.interrupts.Delete(ctx, plan.Run.ID); err != nil {
			return err
		}
		if plan.Run.Outcome == nil {
			return errors.New("bootstrap: terminal run outcome is required")
		}
		switch *plan.Run.Outcome {
		case execution.OutcomeCanceled:
			return s.runs.Terminalize(ctx, plan.Run.SessionID, plan.Run.ID, execution.OutcomeCanceled)
		case execution.OutcomeError:
			return s.runs.RecoverLost(ctx, plan.Run.SessionID, plan.Run.ID)
		default:
			return fmt.Errorf("bootstrap: unsupported parked terminal outcome %s", *plan.Run.Outcome)
		}
	})
}

// deleteInterrupts removes every open-interrupt record and referenced process
// snapshot for a session inside the caller's transaction — the list + per-row
// deletes join the same conn(ctx).
func (s sessionStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if p.ProcessID != "" {
			if err := s.processes.DeleteTree(ctx, p.ProcessID); err != nil {
				return err
			}
		}
		if err := s.interrupts.Delete(ctx, p.RunID); err != nil {
			return err
		}
	}
	return nil
}
