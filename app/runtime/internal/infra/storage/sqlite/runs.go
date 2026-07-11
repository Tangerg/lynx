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

// Terminalize transitions the session's non-terminal Run to terminal with the
// given outcome, freeing the session. Keyed on session (the partial unique index
// guarantees one non-terminal Run, so no run_id is needed and a resumed run whose
// segment id differs still terminalizes), guarded on state so it is idempotent:
// terminalizing an already-terminal / absent session is a no-op.
func (s *RunStateStore) Terminalize(ctx context.Context, sessionID, outcome string) error {
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE runs SET state = ?, outcome = ?, updated_at = ?
		 WHERE session_id = ? AND state != ?`,
		runStateTerminal, outcome, time.Now().UTC().UnixNano(), sessionID, runStateTerminal)
	if err != nil {
		return fmt.Errorf("sqlite: terminalize run: %w", err)
	}
	return nil
}

// ReconcileOrphans terminalizes running Runs abandoned by a crash — their live
// process is gone after restart and no open interrupt keeps them resumable — so
// a crashed run doesn't block its session forever. Interrupted runs (parked, an
// open interrupt recorded) are resumable and left untouched. Run once at boot
// before admitting any run; returns the count swept.
func (s *RunStateStore) ReconcileOrphans(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET state = ?, outcome = ?, updated_at = ?
		 WHERE state = ?
		   AND session_id NOT IN (SELECT DISTINCT session_id FROM interrupts)`,
		runStateTerminal, execution.OutcomeCanceled.String(), time.Now().UTC().UnixNano(), runStateRunning)
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
