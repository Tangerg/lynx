package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// SessionStores is the SQLite-backed adapter for the session lifecycle's
// snapshot and atomic write-set ports. Each operation applies the complete
// application-decided mutation inside one storage transaction.
type SessionStores struct {
	sessions            *sqlitestore.SessionStore
	transcript          *sqlitestore.TranscriptStore
	interrupts          *sqlitestore.InterruptStore
	runs                *sqlitestore.RunStore
	executorCheckpoints *sqlitestore.ExecutorCheckpointStore
	history             *conversation.Messages
	plan                planProjection
	approvals           approvalRuleCleaner
	toolResults         *sqlitestore.ToolResultStore
	goals               goalStore
	tx                  Transactor
}

// SessionStoresConfig is the durable collaborator set for SessionStores.
type SessionStoresConfig struct {
	Sessions            *sqlitestore.SessionStore
	Transcript          *sqlitestore.TranscriptStore
	Interrupts          *sqlitestore.InterruptStore
	Runs                *sqlitestore.RunStore
	ExecutorCheckpoints *sqlitestore.ExecutorCheckpointStore
	History             *conversation.Messages
	Plan                planProjection
	Approvals           approvalRuleCleaner
	ToolResults         *sqlitestore.ToolResultStore
	Goals               goalStore
	Tx                  Transactor
}

// Transactor runs a complete write-set inside one durable transaction.
type Transactor func(context.Context, func(context.Context) error) error

// planProjection is the session-scoped Plan this write-set has to move with
// the session through its whole lifecycle: read for an archive, replaced by a
// restore, seeded into a fork, republished at a rollback boundary, dropped with a
// delete. Replace rather than write-then-set-revision, because the store owns the
// revision — it is assigned by the write itself, so no caller can hand out two
// values under one number.
type planProjection interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
	Replace(ctx context.Context, sessionID string, items []plan.Step) error
	DeleteSession(ctx context.Context, sessionID string) error
}

type approvalRuleCleaner interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

type goalStore interface {
	Clear(ctx context.Context, sessionID string) error
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

// NewSessionStores returns the SQLite adapter for session snapshots and
// write-sets. Its dependencies are assembled once by Bootstrap.
func NewSessionStores(cfg SessionStoresConfig) *SessionStores {
	return &SessionStores{
		sessions:            cfg.Sessions,
		transcript:          cfg.Transcript,
		interrupts:          cfg.Interrupts,
		runs:                cfg.Runs,
		executorCheckpoints: cfg.ExecutorCheckpoints,
		history:             cfg.History,
		plan:                cfg.Plan,
		approvals:           cfg.Approvals,
		toolResults:         cfg.ToolResults,
		goals:               cfg.Goals,
		tx:                  cfg.Tx,
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
		var plan []plan.Step
		if s.plan != nil {
			plan, err = s.plan.List(ctx, sessionID)
			if err != nil {
				return err
			}
		}
		snapshot = sessions.Snapshot{
			Session: ses, Messages: messages, Items: items, Runs: runs,
			ToolResults: toolResults, Plan: plan,
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
		// A branch that copies the conversation copies the plan it was following. Only
		// a non-empty list is written: a fresh child with no row already reads as a
		// session with no list, and writing one would publish an empty list as news.
		if s.plan != nil && len(plan.Plan) > 0 {
			if err := s.plan.Replace(ctx, ch.ID, plan.Plan); err != nil {
				return err
			}
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
		// The boundary's list is REPUBLISHED, not cleared: a rollback is a new state
		// commit, so its value has to arrive under a higher revision than whatever the
		// session already published, and deleting the row would restart that space at
		// one. An unrecorded boundary is left alone — see sessions.PlanBoundary.
		if s.plan != nil && plan.Plan.Recorded {
			if err := s.plan.Replace(ctx, plan.SessionID, plan.Plan.Steps); err != nil {
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
			if err := s.interrupts.Delete(ctx, plan.SessionID, runID); err != nil {
				return err
			}
		}
		if len(plan.CheckpointRootIDs) > 0 {
			if err := s.executorCheckpoints.DeleteCheckpoints(ctx, plan.SessionID, plan.CheckpointRootIDs); err != nil {
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
		// REPLACED after the clear, not restored by it: Replace bumps the projection's
		// revision past whatever this session already published, while the clear left
		// it with no row — and a fresh row starts at one, which a client holding a
		// higher revision would ignore as stale.
		if s.plan != nil {
			if err := s.plan.Replace(ctx, id, plan.Plan); err != nil {
				return err
			}
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

// ApplyDelete removes all durable state for the addressed session.
func (s *SessionStores) ApplyDelete(ctx context.Context, plan sessions.DeletePlan) error {
	if plan.SessionID == "" {
		return errors.New("persistence: delete plan has no session")
	}
	return s.runInTx(ctx, func(ctx context.Context) error {
		return s.deleteSession(ctx, plan.SessionID)
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
	if s.executorCheckpoints != nil {
		if err := s.executorCheckpoints.DeleteSessionCheckpoints(ctx, sessionID); err != nil {
			return err
		}
	}
	if err := s.runs.DeleteForSession(ctx, sessionID); err != nil {
		return err
	}
	if s.plan != nil {
		if err := s.plan.DeleteSession(ctx, sessionID); err != nil {
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
// clears its executor checkpoint atomically.
func (s *SessionStores) ApplyTerminal(ctx context.Context, plan sessions.TerminalPlan) error {
	if err := plan.Validate(); err != nil {
		return fmt.Errorf("persistence: invalid terminal plan: %w", err)
	}
	root, _ := plan.RootRun()
	return s.runInTx(ctx, func(ctx context.Context) error {
		for _, item := range plan.Items {
			if err := s.transcript.AppendItem(ctx, item); err != nil {
				return err
			}
		}
		if plan.CheckpointRootID != "" {
			if err := s.executorCheckpoints.DeleteCheckpoints(ctx, root.SessionID, []string{plan.CheckpointRootID}); err != nil {
				return err
			}
		}
		// The interrupt goes before the terminal write: while it is open, the Run is
		// parked ON it, and a Run cannot be both finished and waiting.
		if err := s.interrupts.Delete(ctx, root.SessionID, root.ID); err != nil {
			return err
		}
		for _, run := range plan.Runs {
			if run.Outcome == nil {
				return fmt.Errorf("persistence: terminal Run %q outcome is required", run.ID)
			}
			switch *run.Outcome {
			case execution.OutcomeCanceled:
				if err := s.runs.Terminalize(ctx, run); err != nil {
					return err
				}
			case execution.OutcomeError:
				if err := s.runs.RecoverLost(ctx, run); err != nil {
					return err
				}
			default:
				return fmt.Errorf("persistence: unsupported parked terminal outcome %s", *run.Outcome)
			}
		}
		if plan.GoalRun != nil {
			if s.goals == nil {
				return errors.New("persistence: Goal Run store is unavailable for a Goal-owned terminal Run")
			}
			if err := s.goals.RecordRun(ctx, *plan.GoalRun); err != nil {
				return fmt.Errorf("persistence: record Goal Run for Run %q: %w", root.ID, err)
			}
		}
		return nil
	})
}

func (s *SessionStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, interrupt := range pending {
		if err := s.interrupts.Delete(ctx, sessionID, interrupt.RootRunID); err != nil {
			return err
		}
	}
	return nil
}
