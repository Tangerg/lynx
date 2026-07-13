package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// Coarse admission states stored in runs.state. The partial unique index
// idx_runs_session_active keys on state != stateTerminal, so a Session holds at
// most one non-terminal Run. The fine [execution.Outcome] is stored separately
// in runs.outcome; the fine [execution.RunState] the domain reasons in projects
// onto these three for the admission constraint.
const (
	runStateRunning     = "running"
	runStateInterrupted = "interrupted"
	runStateTerminal    = "terminal"
)

// RunStateStore is the SQLite-backed authoritative Run admission state (§8.2):
// one row per Run with a partial unique index that guarantees a Session holds at
// most one non-terminal Run across restarts. It is the durable backstop behind
// the in-process live-run registry, which only tracks THIS process's segments.
type RunStateStore struct {
	db *sql.DB
}

// NewRunStateStore binds the run-admission table to db. db must have been opened
// via [Open] so the current schema was installed.
func NewRunStateStore(db *sql.DB) *RunStateStore {
	return &RunStateStore{db: db}
}

// Admit records draft as the session's active (running) Run. It returns
// [execution.ErrSessionBusy] when the partial unique index rejects the INSERT —
// the session already has a non-terminal Run.
func (s *RunStateStore) Admit(ctx context.Context, draft execution.RunDraft) error {
	now := draft.CreatedAt.UTC().UnixNano()
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO runs(run_id, session_id, state, provider, model, outcome, started_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		draft.RunID, draft.SessionID, runStateRunning,
		draft.Provider, draft.Model, now, now)
	if isUniqueViolation(err) {
		return execution.ErrSessionBusy
	}
	if err != nil {
		return fmt.Errorf("sqlite: admit run: %w", err)
	}
	return nil
}

// Suspend parks the exact running Run (Running → Interrupted, kept non-terminal
// so the session stays durably claimed) by deferring to the [execution.RunState]
// machine. A missing row, repeated transition, mismatched identity, or any other
// source state is an ownership error and never succeeds silently.
func (s *RunStateStore) Suspend(ctx context.Context, sessionID, runID string) error {
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		current, found, err := s.stateForRun(ctx, sessionID, runID)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("sqlite: suspend run: active run not found")
		}
		next, ok := current.Suspend()
		if !ok {
			return fmt.Errorf("sqlite: suspend run: illegal transition from %s", current)
		}
		return s.writeState(ctx, sessionID, runID, "suspend", current, next, "")
	})
}

// Resume continues the exact parked Run (Interrupted → Running). Unlike cleanup
// transitions it is strict: a missing/mismatched/already-running row means the
// continuation opening does not own the durable Run and must roll back.
func (s *RunStateStore) Resume(ctx context.Context, draft execution.ResumeDraft) error {
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		cur, found, err := s.stateForRun(ctx, draft.SessionID, draft.RunID)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("sqlite: resume run: active run not found")
		}
		next, ok := cur.Resume()
		if !ok {
			return fmt.Errorf("sqlite: resume run: illegal transition from %s", cur)
		}
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, outcome = '', updated_at = ?
			 WHERE run_id = ? AND session_id = ? AND state = ?`,
			coarseState(next), time.Now().UTC().UnixNano(), draft.RunID, draft.SessionID, coarseState(cur))
		if err != nil {
			return fmt.Errorf("sqlite: resume run: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("sqlite: resume run: state changed concurrently (was %s)", cur)
		}
		return nil
	})
}

func (s *RunStateStore) stateForRun(ctx context.Context, sessionID, runID string) (execution.RunState, bool, error) {
	var coarse string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT state FROM runs WHERE run_id = ? AND session_id = ? AND state != ?`,
		runID, sessionID, runStateTerminal).Scan(&coarse)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("sqlite: read run state: %w", err)
	case coarse == runStateInterrupted:
		return execution.Interrupted, true, nil
	default:
		return execution.Running, true, nil
	}
}

// Terminalize ends the exact non-terminal Run with outcome o, freeing the
// admission slot. RunID + SessionID prevent a late segment from modifying a
// different run admitted by the same session. [execution.RunState.Terminate]
// enforces the machine's legality, and a missing row, repeated transition,
// mismatched identity, or illegal source state fails instead of being hidden.
func (s *RunStateStore) Terminalize(ctx context.Context, sessionID, runID string, o execution.Outcome) error {
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		cur, found, err := s.stateForRun(ctx, sessionID, runID)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("sqlite: terminalize run: active run not found")
		}
		next, ok := cur.Terminate(o)
		if !ok {
			return fmt.Errorf("sqlite: terminalize run: illegal %s from %s", o, cur)
		}
		return s.writeState(ctx, sessionID, runID, "terminalize", cur, next, o.String())
	})
}

// writeState persists a machine-validated transition as a CAS guarded on the
// observed source state; a 0-row result means the row changed under the
// transaction (a lost race) and is surfaced rather than silently dropped. The
// outcome column is stamped from o — empty for a non-terminal transition, which
// leaves it at the empty string every non-terminal row already carries.
func (s *RunStateStore) writeState(ctx context.Context, sessionID, runID, op string, from, to execution.RunState, outcome string) error {
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE runs SET state = ?, outcome = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ?`,
		coarseState(to), outcome, time.Now().UTC().UnixNano(), sessionID, runID, coarseState(from))
	if err != nil {
		return fmt.Errorf("sqlite: %s run: %w", op, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("sqlite: %s run: state changed concurrently (was %s)", op, from)
	}
	return nil
}

// coarseState projects the fine [execution.RunState] onto the three admission
// states the runs table stores — the partial unique index keys on non-terminal,
// so every terminal RunState collapses to the one 'terminal' value (the fine
// terminal reason lives in runs.outcome).
func coarseState(s execution.RunState) string {
	switch {
	case s.IsTerminal():
		return runStateTerminal
	case s == execution.Interrupted:
		return runStateInterrupted
	default:
		return runStateRunning
	}
}

// DeleteForSession drops every Run row of a session whose durable state is being
// removed or replaced wholesale — the session-delete cascade, the import/restore
// replace, and the subagent subtree purge. Freeing the admission slot by deletion
// (not terminalization) keeps the runs table from accumulating dead rows for
// sessions that no longer exist. Joins the caller's transaction via the context.
func (s *RunStateStore) DeleteForSession(ctx context.Context, sessionID string) error {
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM runs WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: delete runs for session: %w", err)
	}
	return nil
}

// ReconcileOrphans terminalizes non-terminal Runs abandoned by a crash — the
// process died with no open interrupt keeping the run resumable — so a crash
// doesn't block its session forever. A non-terminal Run is an orphan iff its
// session has NO open interrupt. The state transition and interrupt open/consume
// are atomic, but both Running continuations and Interrupted parked runs are
// non-terminal; interrupt presence distinguishes the latter at boot. Run once
// before admitting any run; returns the count swept.
func (s *RunStateStore) ReconcileOrphans(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET state = ?, outcome = ?, updated_at = ?
		 WHERE state != ?
		   AND session_id NOT IN (SELECT DISTINCT session_id FROM interrupts)`,
		runStateTerminal, execution.OutcomeCanceled.String(), time.Now().UTC().UnixNano(), runStateTerminal)
	if err != nil {
		return 0, fmt.Errorf("sqlite: reconcile orphan runs: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure —
// the partial-unique-index rejection that means the session already holds a
// non-terminal run. modernc.org/sqlite surfaces it as a typed *sqlite.Error
// carrying the extended result code.
func isUniqueViolation(err error) bool {
	var se *sqlite3.Error
	return errors.As(err, &se) && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE
}
