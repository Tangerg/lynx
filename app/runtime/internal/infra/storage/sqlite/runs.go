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
// via [Open] so the migration ran.
func NewRunStateStore(db *sql.DB) *RunStateStore {
	return &RunStateStore{db: db}
}

// Admit records draft as the session's active (running) Run. It returns
// [execution.ErrSessionBusy] when the partial unique index rejects the INSERT —
// the session already has a non-terminal Run.
func (s *RunStateStore) Admit(ctx context.Context, draft execution.RunDraft) error {
	now := draft.CreatedAt.UTC().UnixNano()
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO runs(run_id, session_id, state, provider, model, outcome, process_id, started_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?, ?)`,
		draft.RunID, draft.SessionID, runStateRunning,
		draft.Provider, draft.Model, draft.ProcessID, now, now)
	if isUniqueViolation(err) {
		return execution.ErrSessionBusy
	}
	if err != nil {
		return fmt.Errorf("sqlite: admit run: %w", err)
	}
	return nil
}

// Suspend parks the session's running Run (Running → Interrupted, kept
// non-terminal so the session stays durably claimed) by deferring to the
// [execution.RunState] machine. Idempotent when the run is already interrupted (a
// re-applied park) or absent (nothing active); Suspend from any other state is
// illegal and surfaces an error rather than silently succeeding.
func (s *RunStateStore) Suspend(ctx context.Context, sessionID string) error {
	return s.advance(ctx, sessionID, "suspend", execution.RunState.Suspend, execution.Interrupted)
}

// Resume continues the session's parked Run (Interrupted → Running) so a crash
// mid-continuation leaves a running row the boot sweep reclaims (the interrupt
// was already consumed) rather than a stuck interrupted one. Idempotent when the
// row is already running (a park's Suspend was missed) or absent; Resume from any
// other state is illegal and surfaced.
func (s *RunStateStore) Resume(ctx context.Context, sessionID string) error {
	return s.advance(ctx, sessionID, "resume", execution.RunState.Resume, execution.Running)
}

// Terminalize ends the session's non-terminal Run with outcome o, freeing the
// admission slot. Keyed on session (the partial unique index guarantees one
// non-terminal Run, so a resumed run whose segment id differs still
// terminalizes). Deferring to [execution.RunState.Terminate] enforces the
// machine's legality — a parked run may terminalize only via cancellation; any
// other terminal from a parked run must resume first — so an illegal terminal
// surfaces instead of silently overwriting the row. Idempotent when the session
// has no non-terminal Run (already terminal / never admitted).
func (s *RunStateStore) Terminalize(ctx context.Context, sessionID string, o execution.Outcome) error {
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		cur, found, err := s.activeState(ctx, sessionID)
		if err != nil || !found {
			return err
		}
		next, ok := cur.Terminate(o)
		if !ok {
			return fmt.Errorf("sqlite: terminalize run: illegal %s from %s", o, cur)
		}
		return s.writeState(ctx, sessionID, "terminalize", cur, next, o.String())
	})
}

// advance applies a domain [execution.RunState] transition to the session's one
// non-terminal Run and persists the result, keeping the store's admission state
// governed by the machine rather than ad-hoc SQL guards:
//
//   - no active run                → benign no-op (already terminal / never admitted).
//   - the move is legal            → CAS-write guarded on the observed state.
//   - illegal but already at dest  → benign idempotent no-op (a re-applied park,
//     or a resume whose park's Suspend was missed).
//   - illegal from any other state → surfaced as an error, not silently dropped.
//
// The read-classify-write runs in one transaction ([RunInTx] joins a caller's
// commit transaction or opens its own), so the observed state can't shift under
// the CAS.
func (s *RunStateStore) advance(ctx context.Context, sessionID, op string, move func(execution.RunState) (execution.RunState, bool), dest execution.RunState) error {
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		cur, found, err := s.activeState(ctx, sessionID)
		if err != nil || !found {
			return err
		}
		next, ok := move(cur)
		if !ok {
			if cur == dest {
				return nil
			}
			return fmt.Errorf("sqlite: %s run: illegal transition from %s", op, cur)
		}
		return s.writeState(ctx, sessionID, op, cur, next, "")
	})
}

// activeState reads the session's single non-terminal Run (the partial unique
// index guarantees at most one) and reconstructs its [execution.RunState].
// found=false means the session has no active run — every transition treats that
// as a benign no-op.
func (s *RunStateStore) activeState(ctx context.Context, sessionID string) (execution.RunState, bool, error) {
	var coarse string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT state FROM runs WHERE session_id = ? AND state != ?`,
		sessionID, runStateTerminal).Scan(&coarse)
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

// writeState persists a machine-validated transition as a CAS guarded on the
// observed source state; a 0-row result means the row changed under the
// transaction (a lost race) and is surfaced rather than silently dropped. The
// outcome column is stamped from o — empty for a non-terminal transition, which
// leaves it at the empty string every non-terminal row already carries.
func (s *RunStateStore) writeState(ctx context.Context, sessionID, op string, from, to execution.RunState, outcome string) error {
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE runs SET state = ?, outcome = ?, updated_at = ?
		 WHERE session_id = ? AND state = ?`,
		coarseState(to), outcome, time.Now().UTC().UnixNano(), sessionID, coarseState(from))
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
// session has NO open interrupt: this is robust to a best-effort Suspend/Resume
// that failed to advance the row's state (a parked run whose Suspend was missed
// stays 'running' but its interrupt still preserves it; a continuation whose
// Resume was missed stays 'interrupted' but its consumed interrupt no longer
// does, so the sweep reclaims it). Once §8.3 commits the interrupt open/close in
// the same transaction as the state transition, the state alone is authoritative
// and this can key on state = 'running'. Run once at boot before admitting any
// run; returns the count swept.
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
