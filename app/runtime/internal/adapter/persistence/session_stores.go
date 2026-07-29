package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// SessionStores is the SQLite-backed adapter for the session lifecycle's
// snapshot and atomic write-set ports. Each operation applies the complete
// application-decided mutation inside one storage transaction.
type SessionStores struct {
	sessions    *sqlitestore.SessionStore
	transcript  *sqlitestore.TranscriptStore
	interrupts  *sqlitestore.InterruptStore
	runs        *sqlitestore.RunStore
	processes   *sqlitestore.ProcessStore
	history     *conversation.Messages
	todos       todoCleaner
	approvals   approvalRuleCleaner
	toolResults *sqlitestore.ToolResultStore
	goals       goalCleaner
	tx          Transactor
}

// SessionStoresConfig is the durable collaborator set for SessionStores.
type SessionStoresConfig struct {
	Sessions    *sqlitestore.SessionStore
	Transcript  *sqlitestore.TranscriptStore
	Interrupts  *sqlitestore.InterruptStore
	Runs        *sqlitestore.RunStore
	Processes   *sqlitestore.ProcessStore
	History     *conversation.Messages
	Todos       todoCleaner
	Approvals   approvalRuleCleaner
	ToolResults *sqlitestore.ToolResultStore
	Goals       goalCleaner
	Tx          Transactor
}

// Transactor runs a complete write-set inside one durable transaction.
type Transactor func(context.Context, func(context.Context) error) error

type todoCleaner interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

type approvalRuleCleaner interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

type goalCleaner interface {
	Clear(ctx context.Context, sessionID string) error
}

// NewSessionStores returns the SQLite adapter for session snapshots and
// write-sets. Its dependencies are assembled once by Bootstrap.
func NewSessionStores(cfg SessionStoresConfig) *SessionStores {
	return &SessionStores{
		sessions:    cfg.Sessions,
		transcript:  cfg.Transcript,
		interrupts:  cfg.Interrupts,
		runs:        cfg.Runs,
		processes:   cfg.Processes,
		history:     cfg.History,
		todos:       cfg.Todos,
		approvals:   cfg.Approvals,
		toolResults: cfg.ToolResults,
		goals:       cfg.Goals,
		tx:          cfg.Tx,
	}
}

var _ sessions.SnapshotReader = (*SessionStores)(nil)
var _ sessions.WriteSets = (*SessionStores)(nil)

func (s *SessionStores) ReadSnapshot(ctx context.Context, sessionID string) (sessions.Snapshot, error) {
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
		items, err := s.transcript.List(ctx, sessionID)
		if err != nil {
			return err
		}
		runs, err := s.runs.ListRuns(ctx, sessionID)
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
		return nil
	})
	return snapshot, err
}

func (s *SessionStores) runInTx(ctx context.Context, fn func(context.Context) error) error {
	return s.tx(ctx, fn)
}

// ApplyFork branches a child session, seeds its history prefix, and applies its
// title in one transaction.
func (s *SessionStores) ApplyFork(ctx context.Context, plan sessions.ForkPlan) (session.Session, error) {
	var child session.Session
	err := s.runInTx(ctx, func(ctx context.Context) error {
		ch, err := s.sessions.Fork(ctx, plan.ParentID)
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

// ApplyRollback persists one resolved rollback plan atomically.
func (s *SessionStores) ApplyRollback(ctx context.Context, plan sessions.RollbackPlan) error {
	return s.runInTx(ctx, func(ctx context.Context) error {
		if s.todos != nil {
			if err := s.todos.DeleteSession(ctx, plan.SessionID); err != nil {
				return err
			}
		}
		if s.goals != nil {
			if err := s.goals.Clear(ctx, plan.SessionID); err != nil {
				return err
			}
		}
		if plan.KeepMark >= 0 {
			if err := s.history.Truncate(ctx, plan.SessionID, plan.KeepMark); err != nil {
				return err
			}
		}
		for _, runID := range plan.DropRunIDs {
			if err := s.transcript.DeleteRun(ctx, plan.SessionID, runID); err != nil {
				return err
			}
			// A dropped Run ceases to exist, which is also how it releases the
			// session's admission slot — there is no state left to terminalize.
			if err := s.runs.Delete(ctx, plan.SessionID, runID); err != nil {
				return err
			}
			if err := s.interrupts.Delete(ctx, runID); err != nil {
				return err
			}
		}
		if len(plan.ProcessIDs) > 0 {
			if err := s.processes.DeleteTrees(ctx, plan.ProcessIDs); err != nil {
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

// ApplyRestore replaces every durable projection for a restored session in one
// transaction.
func (s *SessionStores) ApplyRestore(ctx context.Context, plan sessions.RestorePlan) error {
	id := plan.Session.ID
	if s.toolResults == nil && len(plan.ToolResults) > 0 {
		return errors.New("persistence: cannot restore tool results without blob persistence")
	}
	return s.runInTx(ctx, func(ctx context.Context) error {
		if err := s.sessions.Restore(ctx, plan.Session); err != nil {
			return err
		}
		if err := s.clearSessionOwnedState(ctx, id); err != nil {
			return err
		}
		if err := s.history.Seed(ctx, id, plan.Messages); err != nil {
			return err
		}
		for _, run := range plan.Runs {
			if err := s.runs.Restore(ctx, run); err != nil {
				return err
			}
		}
		for _, item := range plan.Items {
			if err := s.transcript.AppendItem(ctx, item); err != nil {
				return err
			}
		}
		if s.toolResults != nil {
			for _, blob := range plan.ToolResults {
				if err := s.toolResults.Restore(ctx, blob); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ApplyDelete removes all durable state for the requested session cascade.
func (s *SessionStores) ApplyDelete(ctx context.Context, plan sessions.DeletePlan) error {
	if len(plan.SessionIDs) == 0 {
		return errors.New("persistence: delete plan has no sessions")
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

func (s *SessionStores) deleteSession(ctx context.Context, sessionID string) error {
	if err := s.clearSessionOwnedState(ctx, sessionID); err != nil {
		return err
	}
	return s.sessions.Delete(ctx, sessionID)
}

func (s *SessionStores) clearSessionOwnedState(ctx context.Context, sessionID string) error {
	if err := s.transcript.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.history.Clear(ctx, sessionID); err != nil {
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
	if s.goals != nil {
		if err := s.goals.Clear(ctx, sessionID); err != nil {
			return err
		}
	}
	if s.toolResults != nil {
		if err := s.toolResults.DropSession(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

// ApplyTerminal persists the terminal record for an abandoned parked run and
// clears its resumable process state atomically.
func (s *SessionStores) ApplyTerminal(ctx context.Context, plan sessions.TerminalPlan) error {
	return s.runInTx(ctx, func(ctx context.Context) error {
		for _, item := range plan.Items {
			if err := s.transcript.AppendItem(ctx, item); err != nil {
				return err
			}
		}
		if plan.ProcessID != "" {
			if err := s.processes.DeleteTrees(ctx, []string{plan.ProcessID}); err != nil {
				return err
			}
		}
		// The interrupt goes before the terminal write: while it is open, the Run is
		// parked ON it, and a Run cannot be both finished and waiting.
		if err := s.interrupts.Delete(ctx, plan.Run.ID); err != nil {
			return err
		}
		if plan.Run.Outcome == nil {
			return errors.New("persistence: terminal run outcome is required")
		}
		switch *plan.Run.Outcome {
		case execution.OutcomeCanceled:
			return s.runs.Terminalize(ctx, plan.Run)
		case execution.OutcomeError:
			return s.runs.RecoverLost(ctx, plan.Run)
		default:
			return fmt.Errorf("persistence: unsupported parked terminal outcome %s", *plan.Run.Outcome)
		}
	})
}

func (s *SessionStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	processIDs := make([]string, 0, len(pending))
	for _, interrupt := range pending {
		if interrupt.ProcessID != "" {
			processIDs = append(processIDs, interrupt.ProcessID)
		}
	}
	if len(processIDs) > 0 {
		if err := s.processes.DeleteTrees(ctx, processIDs); err != nil {
			return err
		}
	}
	for _, interrupt := range pending {
		if err := s.interrupts.Delete(ctx, interrupt.RootRunID); err != nil {
			return err
		}
	}
	return nil
}
