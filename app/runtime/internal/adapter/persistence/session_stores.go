package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/conversations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// SessionStores is the SQLite-backed adapter for the session lifecycle's
// snapshot and atomic write-set ports. Each operation applies the complete
// application-decided mutation inside one storage transaction.
type SessionStores struct {
	sessions            *sqlitestore.SessionStore
	transcript          *sqlitestore.TranscriptStore
	interrupts          *InterruptStore
	runs                *sqlitestore.RunStore
	executorCheckpoints *ExecutorCheckpointStore
	history             *conversations.Messages
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
	Interrupts          *InterruptStore
	Runs                *sqlitestore.RunStore
	ExecutorCheckpoints *ExecutorCheckpointStore
	History             *conversations.Messages
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
// write-sets. Its dependencies are fixed at construction.
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
	err := s.tx(ctx, func(ctx context.Context) error {
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
		var toolResults []toolresult.Blob
		if s.toolResults != nil {
			toolResults, err = s.toolResults.List(ctx, sessionID)
			if err != nil {
				return err
			}
		}
		var steps []plan.Step
		if s.plan != nil {
			steps, err = s.plan.List(ctx, sessionID)
			if err != nil {
				return err
			}
		}
		snapshot = sessions.Snapshot{
			Session: ses, Messages: messages, Items: items, Runs: runs,
			ToolResults: toolResults, Plan: steps,
		}
		return nil
	})
	return snapshot, err
}

// ApplyFork branches a child session, seeds its history prefix, and applies its
// title in one transaction.
func (s *SessionStores) ApplyFork(ctx context.Context, fork sessions.ForkPlan) (session.Session, error) {
	var child session.Session
	err := s.tx(ctx, func(ctx context.Context) error {
		ch, err := s.sessions.Fork(ctx, fork.ParentID)
		if err != nil {
			return err
		}
		if err := s.history.Seed(ctx, ch.ID, fork.Messages); err != nil {
			return err
		}
		// A branch that copies the conversation copies the plan it was following. Only
		// a non-empty list is written: a fresh child with no row already reads as a
		// session with no list, and writing one would publish an empty list as news.
		if s.plan != nil && len(fork.Plan) > 0 {
			if err := s.plan.Replace(ctx, ch.ID, fork.Plan); err != nil {
				return err
			}
		}
		if fork.Title != "" {
			if err := s.sessions.Rename(ctx, ch.ID, fork.Title); err != nil {
				return err
			}
			ch.Title = fork.Title
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
func (s *SessionStores) ApplyRollback(ctx context.Context, rollback sessions.RollbackPlan) error {
	return s.tx(ctx, func(ctx context.Context) error {
		if err := s.republishRollbackState(ctx, rollback); err != nil {
			return err
		}
		if err := s.deleteRolledBackRuns(ctx, rollback.SessionID, rollback.DropRunIDs); err != nil {
			return err
		}
		if len(rollback.CheckpointRootIDs) > 0 {
			if err := s.executorCheckpoints.DeleteCheckpoints(ctx, rollback.SessionID, rollback.CheckpointRootIDs); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SessionStores) republishRollbackState(ctx context.Context, rollback sessions.RollbackPlan) error {
	// The boundary's list is republished, not cleared: rollback is a new state
	// commit, so its value must arrive under a higher revision than the session's
	// prior value. Deleting the row would restart that revision space at one. An
	// unrecorded boundary is left alone; see sessions.PlanBoundary.
	if s.plan != nil && rollback.Plan.Recorded {
		if err := s.plan.Replace(ctx, rollback.SessionID, rollback.Plan.Steps); err != nil {
			return err
		}
	}
	if s.goals != nil {
		if err := s.goals.Clear(ctx, rollback.SessionID); err != nil {
			return err
		}
	}
	if rollback.KeepMessageMark >= 0 {
		return s.history.Truncate(ctx, rollback.SessionID, rollback.KeepMessageMark)
	}
	return nil
}

func (s *SessionStores) deleteRolledBackRuns(ctx context.Context, sessionID string, runIDs []string) error {
	for _, runID := range runIDs {
		if err := s.transcript.DeleteRun(ctx, sessionID, runID); err != nil {
			return err
		}
		// A rolled-back Run ceases to exist and therefore releases the Session's
		// admission slot; no state remains to terminalize.
		if err := s.runs.Delete(ctx, sessionID, runID); err != nil {
			return err
		}
		if err := s.interrupts.Delete(ctx, sessionID, runID); err != nil {
			return err
		}
	}
	return nil
}

// ApplyRestore replaces every durable projection for a restored session in one
// transaction.
func (s *SessionStores) ApplyRestore(ctx context.Context, restore sessions.RestorePlan) error {
	if s.toolResults == nil && len(restore.ToolResults) > 0 {
		return errors.New("persistence: cannot restore tool results without blob persistence")
	}
	return s.tx(ctx, func(ctx context.Context) error {
		sessionID := restore.Session.ID
		if err := s.sessions.Restore(ctx, restore.Session); err != nil {
			return err
		}
		if err := s.clearSessionOwnedState(ctx, sessionID); err != nil {
			return err
		}
		if err := s.restorePlanAndHistory(ctx, sessionID, restore); err != nil {
			return err
		}
		if err := s.restoreRuns(ctx, restore.Runs); err != nil {
			return err
		}
		if err := s.appendTranscriptItems(ctx, restore.Items); err != nil {
			return err
		}
		return s.restoreToolResults(ctx, restore.ToolResults)
	})
}

func (s *SessionStores) restorePlanAndHistory(ctx context.Context, sessionID string, restore sessions.RestorePlan) error {
	// Replace after the clear so the Plan revision advances past the value that
	// clients may already hold. Recreating a deleted row at revision one would be
	// ignored as stale.
	if s.plan != nil {
		if err := s.plan.Replace(ctx, sessionID, restore.Plan); err != nil {
			return err
		}
	}
	return s.history.Seed(ctx, sessionID, restore.Messages)
}

func (s *SessionStores) restoreRuns(ctx context.Context, restored []rundomain.Run) error {
	for _, run := range restored {
		if err := s.runs.Restore(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStores) restoreToolResults(ctx context.Context, blobs []toolresult.Blob) error {
	if s.toolResults == nil {
		return nil
	}
	for _, blob := range blobs {
		if err := s.toolResults.Restore(ctx, blob); err != nil {
			return err
		}
	}
	return nil
}

// ApplyDelete removes all durable state for the addressed session.
func (s *SessionStores) ApplyDelete(ctx context.Context, deletion sessions.DeletePlan) error {
	if deletion.SessionID == "" {
		return errors.New("persistence: delete plan has no session")
	}
	return s.tx(ctx, func(ctx context.Context) error {
		return s.deleteSession(ctx, deletion.SessionID)
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
func (s *SessionStores) ApplyTerminal(ctx context.Context, terminal sessions.TerminalPlan) error {
	if err := terminal.Validate(); err != nil {
		return fmt.Errorf("persistence: invalid terminal plan: %w", err)
	}
	root, _ := terminal.RootRun()
	return s.tx(ctx, func(ctx context.Context) error {
		if err := s.appendTranscriptItems(ctx, terminal.Items); err != nil {
			return err
		}
		if err := s.clearParkedRunState(ctx, root, terminal.CheckpointRootID); err != nil {
			return err
		}
		if err := s.terminalizeParkedRuns(ctx, terminal.Runs); err != nil {
			return err
		}
		return s.recordGoalTerminalRun(ctx, root.ID(), terminal.GoalRun)
	})
}

func (s *SessionStores) appendTranscriptItems(ctx context.Context, items []transcript.Item) error {
	for _, item := range items {
		if err := s.transcript.AppendItem(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStores) clearParkedRunState(ctx context.Context, root rundomain.Run, checkpointRootID string) error {
	if checkpointRootID != "" {
		if err := s.executorCheckpoints.DeleteCheckpoints(ctx, root.SessionID(), []string{checkpointRootID}); err != nil {
			return err
		}
	}
	// Delete the interrupt before the terminal write: while it exists the Run is
	// parked on it, and a Run cannot be both finished and waiting.
	return s.interrupts.Delete(ctx, root.SessionID(), root.ID())
}

func (s *SessionStores) terminalizeParkedRuns(ctx context.Context, runs []rundomain.Run) error {
	for _, run := range runs {
		outcome, terminal := run.Outcome()
		if !terminal {
			return fmt.Errorf("persistence: terminal Run %q outcome is required", run.ID())
		}
		switch outcome {
		case rundomain.OutcomeCanceled:
			if err := s.runs.Terminalize(ctx, run); err != nil {
				return err
			}
		case rundomain.OutcomeLost:
			if err := s.runs.RecoverLost(ctx, run); err != nil {
				return err
			}
		default:
			return fmt.Errorf("persistence: unsupported parked terminal outcome %s", outcome)
		}
	}
	return nil
}

func (s *SessionStores) recordGoalTerminalRun(ctx context.Context, rootRunID string, record *goal.RunRecord) error {
	if record == nil {
		return nil
	}
	if s.goals == nil {
		return errors.New("persistence: Goal Run store is unavailable for a Goal-owned terminal Run")
	}
	if err := s.goals.RecordRun(ctx, *record); err != nil {
		return fmt.Errorf("persistence: record Goal Run for Run %q: %w", rootRunID, err)
	}
	return nil
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
