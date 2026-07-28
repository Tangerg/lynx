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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
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

// RunStore is the SQLite-backed Run table: one row per Run, holding the whole
// Run. Its state column carries the coarse admission position under a partial
// unique index that guarantees a Session holds at most one non-terminal Run
// across restarts — the durable backstop behind the in-process live-run
// registry, which only tracks THIS process's segments — and the rest of the row
// carries what the Run accrued.
//
// One table, one owner: the accrued facts are written only by the lifecycle
// transition that makes them true, so "where is this Run" and "how did it end"
// cannot disagree. A Run's open interrupts are the one part kept elsewhere: the
// interrupts table owns them and reads compose them.
type RunStore struct {
	db *sql.DB
}

// NewRunStore binds the Run table to db. db must have been opened via [Open] so
// the current schema was installed.
func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

// Admit records draft as the session's active (running) Run. It returns
// [execution.ErrSessionBusy] when the partial unique index rejects the INSERT —
// the session already has a non-terminal Run — and
// [transcript.ErrIdentityConflict] when the run id is already taken, since the
// caller may supply one.
func (s *RunStore) Admit(ctx context.Context, draft execution.RunDraft) error {
	// started_at is what orders every Run read, and a zero time stores as a
	// nonsense instant rather than an obviously missing one — so it is refused
	// here instead of quietly sorting Runs by an accident.
	if draft.CreatedAt.IsZero() {
		return fmt.Errorf("sqlite: admit run %q: admission time is required", draft.RunID)
	}
	now := draft.CreatedAt.UTC().UnixNano()
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO runs(run_id, session_id, state, provider, model, message_mark, started_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		draft.RunID, draft.SessionID, runStateRunning,
		draft.ModelSelection.Provider(), draft.ModelSelection.Model(),
		transcript.UnknownMessageMark, now, now)
	// Two constraints can reject this INSERT and they mean opposite things: the
	// primary key says the id is spoken for, the partial index says the session is.
	// Reporting the wrong one would tell a caller to wait for a run to finish when
	// the real answer is to pick another id.
	switch {
	case isPrimaryKeyViolation(err):
		return fmt.Errorf("%w: run %q already exists", transcript.ErrIdentityConflict, draft.RunID)
	case isUniqueViolation(err):
		return execution.ErrSessionBusy
	case err != nil:
		return fmt.Errorf("sqlite: admit run: %w", err)
	}
	return nil
}

// Suspend parks the exact running Run (Running → Interrupted, kept non-terminal
// so the session stays durably claimed) by deferring to the [execution.RunState]
// machine. A missing row, repeated transition, mismatched identity, or any other
// source state is an ownership error and never succeeds silently.
func (s *RunStore) Suspend(ctx context.Context, sessionID, runID string) error {
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
		return s.writeState(ctx, sessionID, runID, "suspend", current, next)
	})
}

// Resume continues the exact parked Run (Interrupted → Running). Unlike cleanup
// transitions it is strict: a missing/mismatched/already-running row means the
// continuation opening does not own the durable Run and must roll back.
func (s *RunStore) Resume(ctx context.Context, draft execution.ResumeDraft) error {
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
		return s.writeState(ctx, draft.SessionID, draft.RunID, "resume", cur, next)
	})
}

// Terminalize ends the exact non-terminal Run that run identifies, recording the
// outcome the executor reached and the result that explains it.
func (s *RunStore) Terminalize(ctx context.Context, run transcript.Run) error {
	if run.Outcome == nil {
		return fmt.Errorf("sqlite: terminalize run %q: outcome is required", run.ID)
	}
	outcome := *run.Outcome
	return s.finish(ctx, "terminalize", run, func(cur execution.RunState) (execution.RunState, bool) {
		return cur.Terminate(outcome)
	})
}

// RecoverLost ends the exact non-terminal Run whose executor state is no longer
// resumable. Unlike Terminalize, this recovery transition is legal from either
// Running or Interrupted, because it describes a Run nobody is driving rather
// than one the executor finished.
func (s *RunStore) RecoverLost(ctx context.Context, run transcript.Run) error {
	return s.finish(ctx, "recover lost", run, execution.RunState.RecoverLost)
}

// finish ends a non-terminal Run, writing the terminal state, its reason, and the
// facts that explain it in ONE statement — a row can never claim a terminal
// state without the result behind it, nor hold a result while still running.
// authorize is the state machine's rule for this kind of ending; the UPDATE is a
// CAS on the state it was authorized from, so a row that moved under the
// transaction fails instead of being overwritten.
func (s *RunStore) finish(
	ctx context.Context,
	op string,
	run transcript.Run,
	authorize func(execution.RunState) (execution.RunState, bool),
) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, run.ID, err)
	}
	row, err := terminalRunRow(run)
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, run.ID, err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		cur, found, err := s.stateForRun(ctx, run.SessionID, run.ID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("sqlite: %s run: active run not found", op)
		}
		next, ok := authorize(cur)
		if !ok {
			return fmt.Errorf("sqlite: %s run: illegal transition from %s", op, cur)
		}
		if next != run.State {
			return fmt.Errorf("sqlite: %s run: %s reaches %s, not the recorded %s", op, cur, next, run.State)
		}
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE runs SET
			   state = ?, outcome = ?, detail = ?, steps = ?, active_duration_ns = ?,
			   usage = ?, problem = ?, message_mark = ?, finished_at = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next), run.Outcome.String(), run.Detail, row.steps, row.durationNs,
			row.usage, row.problem, run.MessageMark, run.FinishedAt.UTC().UnixNano(),
			runUpdatedAt(run), run.SessionID, run.ID, coarseState(cur))
		if err != nil {
			return fmt.Errorf("sqlite: %s run: %w", op, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("sqlite: %s run: state changed concurrently (was %s)", op, cur)
		}
		return nil
	})
}

// Restore inserts a complete terminal Run row for a session being imported or
// restored. It is not an admission: an imported Run has already finished, so it
// never claims the session's non-terminal slot and never passes through the
// state machine. A non-terminal Run is refused — restoring one would hand the
// session's admission slot to an executor that is not running.
func (s *RunStore) Restore(ctx context.Context, run transcript.Run) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", run.ID, err)
	}
	if !run.State.IsTerminal() {
		return fmt.Errorf("sqlite: restore run %q: state is %s, want terminal", run.ID, run.State)
	}
	row, err := terminalRunRow(run)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", run.ID, err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO runs(
		   run_id, session_id, spawned_by_item_id, state, outcome, provider, model,
		   detail, steps, active_duration_ns, usage, problem, message_mark,
		   started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.SessionID, run.SpawnedByItemID, coarseState(run.State), run.Outcome.String(),
		run.ModelSelection.Provider(), run.ModelSelection.Model(),
		run.Detail, row.steps, row.durationNs, row.usage, row.problem, run.MessageMark,
		run.CreatedAt.UTC().UnixNano(), run.FinishedAt.UTC().UnixNano(), runUpdatedAt(run))
	if isPrimaryKeyViolation(err) {
		// A Run id belongs to one Session for its whole lifetime. An import that
		// would re-parent an existing Run is refused rather than silently taking it
		// over, which is what an upsert here would do.
		return fmt.Errorf("%w: run %q already exists", transcript.ErrIdentityConflict, run.ID)
	}
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", run.ID, err)
	}
	return nil
}

func (s *RunStore) stateForRun(ctx context.Context, sessionID, runID string) (execution.RunState, bool, error) {
	var coarse string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT state FROM runs WHERE run_id = ? AND session_id = ? AND state != ?`,
		runID, sessionID, runStateTerminal).Scan(&coarse)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("sqlite: read run state: %w", err)
	case coarse == runStateRunning:
		return execution.Running, true, nil
	case coarse == runStateInterrupted:
		return execution.Interrupted, true, nil
	default:
		return 0, false, fmt.Errorf("sqlite: read run state: unknown state %q", coarse)
	}
}

// ListRunning returns Runs in the running state, oldest admission first, scoped
// to sessionID when it is non-empty. Parked and terminal Runs are excluded: a
// caller asking what is executing is asking about work in progress, and an
// interrupted Run is waiting on a person.
//
// The page is bounded here rather than by the caller. after is the (admission
// time, run id) position a previous page ended at — zero time starts at the
// beginning — and the pair is what makes the order total, since two Runs in
// different sessions can be admitted in the same nanosecond.
func (s *RunStore) ListRunning(ctx context.Context, sessionID string, afterStartedAt int64, afterRunID string, limit int) ([]execution.AdmittedRun, error) {
	query := `SELECT run_id, session_id, provider, model, started_at
		 FROM runs WHERE state = ?`
	args := []any{runStateRunning}
	if sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	if afterStartedAt > 0 || afterRunID != "" {
		query += ` AND (started_at > ? OR (started_at = ? AND run_id > ?))`
		args = append(args, afterStartedAt, afterStartedAt, afterRunID)
	}
	query += ` ORDER BY started_at, run_id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list running runs: %w", err)
	}
	defer rows.Close()

	var out []execution.AdmittedRun
	for rows.Next() {
		var run execution.AdmittedRun
		var provider, model string
		var startedAt int64
		if err := rows.Scan(&run.RunID, &run.SessionID, &provider, &model, &startedAt); err != nil {
			return nil, fmt.Errorf("sqlite: list running runs: %w", err)
		}
		run.State = execution.Running
		// A half-set pair is a corrupt row, not a run without a selection: the
		// admission that wrote it took both values from one Selection.
		run.ModelSelection, err = modelref.New(provider, model)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list running runs: run %q: %w", run.RunID, err)
		}
		run.StartedAt = time.Unix(0, startedAt).UTC()
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list running runs: %w", err)
	}
	return out, nil
}

// runColumns is the whole Run row, joined with the open interrupts a parked Run
// is waiting on — kept in the interrupts table so one park is one record.
const runColumns = `r.run_id, r.session_id, r.spawned_by_item_id, r.state, r.outcome,
	r.provider, r.model, r.detail, r.steps, r.active_duration_ns, r.usage, r.problem,
	r.message_mark, r.started_at, r.finished_at, r.updated_at, i.payload`

// ListRuns returns a session's Runs in admission order, each as the complete
// aggregate: its lifecycle position, the facts it accrued, and — while parked —
// the interrupts it is waiting on.
func (s *RunStore) ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 LEFT JOIN interrupts AS i ON i.run_id = r.run_id AND i.session_id = r.session_id
		 WHERE r.session_id = ? ORDER BY r.started_at, r.run_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list runs: %w", err)
	}
	defer rows.Close()

	var out []transcript.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list runs: %w", err)
	}
	return out, nil
}

// writeState persists a machine-validated non-terminal transition as a CAS
// guarded on the observed source state; a 0-row result means the row changed
// under the transaction (a lost race) and is surfaced rather than silently
// dropped. Terminal transitions do not come through here — they carry the facts
// that explain them and go through finish.
func (s *RunStore) writeState(ctx context.Context, sessionID, runID, op string, from, to execution.RunState) error {
	if to.IsTerminal() {
		return fmt.Errorf("sqlite: %s run: %s is terminal and must carry a result", op, to)
	}
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE runs SET state = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ?`,
		coarseState(to), time.Now().UTC().UnixNano(), sessionID, runID, coarseState(from))
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

// Delete drops one Run's row. The rollback boundary uses it: a Run being dropped
// wholesale frees the session's admission slot by ceasing to exist, so there is
// nothing left to terminalize.
func (s *RunStore) Delete(ctx context.Context, sessionID, runID string) error {
	if sessionID == "" || runID == "" {
		return errors.New("sqlite: delete run requires sessionId + runId")
	}
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM runs WHERE run_id = ? AND session_id = ?`, runID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete run: %w", err)
	}
	return nil
}

// DeleteForSession drops every Run row of a session whose durable state is being
// removed or replaced wholesale — the session-delete cascade, the import/restore
// replace, and the subagent subtree purge. Freeing the admission slot by deletion
// (not terminalization) keeps the runs table from accumulating dead rows for
// sessions that no longer exist. Joins the caller's transaction via the context.
func (s *RunStore) DeleteForSession(ctx context.Context, sessionID string) error {
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM runs WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: delete runs for session: %w", err)
	}
	return nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-index failure — for
// this table, the partial-unique-index rejection that means the session already
// holds a non-terminal run. A primary-key collision has its OWN extended code and
// does NOT appear here, which is what lets an id clash be told apart from a busy
// session. modernc.org/sqlite surfaces both as a typed *sqlite.Error carrying the
// extended result code.
func isUniqueViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE
}

// isPrimaryKeyViolation reports whether err is a SQLite PRIMARY KEY collision —
// here, a run id that already belongs to a Run.
func isPrimaryKeyViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_PRIMARYKEY
}

// runUpdatedAt is the row's touch time. A caller that recorded one is honored so
// a restored Run keeps the timestamp it was exported with; otherwise the write
// stamps itself.
func runUpdatedAt(run transcript.Run) int64 {
	if run.UpdatedAt.IsZero() {
		return time.Now().UTC().UnixNano()
	}
	return run.UpdatedAt.UTC().UnixNano()
}
