package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
	if draft.SegmentID == "" {
		return fmt.Errorf("sqlite: admit run %q: opening segment is required", draft.RunID)
	}
	if err := draft.Limits.Validate(); err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	profile, err := encodeRunProtocolProfile(draft.ProtocolProfile)
	if err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	now := draft.CreatedAt.UTC().UnixNano()
	// This is the profile's only writer, here and in Restore. Suspend / Resume /
	// finish deliberately do not name the column: the Run's contract cannot change
	// after admission, and the way to guarantee that is to have nothing able to
	// change it.
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO runs(run_id, session_id, state, active_segment_id, provider, model, max_steps, max_budget_usd, protocol_profile, message_mark, started_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		draft.RunID, draft.SessionID, runStateRunning, draft.SegmentID,
		draft.ModelSelection.Provider(), draft.ModelSelection.Model(),
		draft.Limits.MaxSteps, draft.Limits.MaxBudgetUSD, profile,
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
// machine, recording what the Run had consumed up to the park. A missing row,
// repeated transition, mismatched identity, or any other source state is an
// ownership error and never succeeds silently.
func (s *RunStore) Suspend(ctx context.Context, run transcript.Run) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("sqlite: suspend run %q: %w", run.ID, err)
	}
	metrics, err := runMetricsRow(run.Metrics)
	if err != nil {
		return fmt.Errorf("sqlite: suspend run %q: %w", run.ID, err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		current, found, err := s.stateForRun(ctx, run.SessionID, run.ID)
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
		if next != run.State {
			return fmt.Errorf("sqlite: suspend run: %s reaches %s, not the recorded %s", current, next, run.State)
		}
		// The segment identity is cleared in the same statement that parks the Run:
		// a Run waiting on a person has no segment to attach to.
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = '', steps = ?, active_duration_ns = ?, usage = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next), metrics.steps, metrics.durationNs, metrics.usage, runUpdatedAt(run),
			run.SessionID, run.ID, coarseState(current))
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("sqlite: suspend run: state changed concurrently (was %s)", current)
		}
		return nil
	})
}

// Resume continues the exact parked Run (Interrupted → Running). Unlike cleanup
// transitions it is strict: a missing/mismatched/already-running row means the
// continuation opening does not own the durable Run and must roll back.
func (s *RunStore) Resume(ctx context.Context, draft execution.ResumeDraft) error {
	if draft.SegmentID == "" {
		return fmt.Errorf("sqlite: resume run %q: continuation segment is required", draft.RunID)
	}
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
		// The accrual is untouched: a continuation inherits what the park committed,
		// and the segment now opening has consumed nothing yet. What does move is the
		// segment identity, which the park cleared and this one replaces.
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next), draft.SegmentID, time.Now().UTC().UnixNano(),
			draft.SessionID, draft.RunID, coarseState(cur))
		if err != nil {
			return fmt.Errorf("sqlite: resume run: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("sqlite: resume run: state changed concurrently (was %s)", cur)
		}
		return nil
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
	metrics, err := runMetricsRow(run.Metrics)
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, run.ID, err)
	}
	problem, err := encodeRunProblem(run.Error)
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
			   state = ?, active_segment_id = '', outcome = ?, detail = ?, steps = ?, active_duration_ns = ?,
			   usage = ?, problem = ?, message_mark = ?, finished_at = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next), run.Outcome.String(), run.Detail, metrics.steps, metrics.durationNs,
			metrics.usage, problem, run.MessageMark, run.FinishedAt.UTC().UnixNano(),
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
	metrics, err := runMetricsRow(run.Metrics)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", run.ID, err)
	}
	problem, err := encodeRunProblem(run.Error)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", run.ID, err)
	}
	profile, err := encodeRunProtocolProfile(run.ProtocolProfile)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", run.ID, err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO runs(
		   run_id, session_id, spawned_by_item_id, state, outcome, provider, model,
		   detail, steps, active_duration_ns, usage, problem, max_steps, max_budget_usd,
		   protocol_profile, message_mark, started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.SessionID, run.SpawnedByItemID, coarseState(run.State), run.Outcome.String(),
		run.ModelSelection.Provider(), run.ModelSelection.Model(),
		run.Detail, metrics.steps, metrics.durationNs, metrics.usage, problem,
		run.Limits.MaxSteps, run.Limits.MaxBudgetUSD, profile, run.MessageMark,
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

// PageRuns returns one page of the root Runs a caller may browse, newest
// admission first, scoped to sessionID when it is non-empty and to statuses when
// it is non-empty. It is the whole history, not the work in progress: a Run that
// ended is still the answer to "what did this session cost", and a Run parked on
// a person is the one a client most needs to find.
//
// Only root Runs are returned. A Run spawned by an item belongs to the subtree of
// the Run that spawned it, and is reached through that Run — so the predicate is
// "has no spawning item" rather than the absence of children in this build.
//
// The page is bounded here rather than by the caller. before is the (admission
// time, run id) position a previous page ended at, applied only when the run id
// is present — every anchor carries both halves, so an empty id is the first page
// and not a Run admitted at the epoch. The pair is what makes the order total,
// since two Runs in different sessions can be admitted in the same nanosecond.
//
// It returns whole Runs rather than a thinner admission projection, because the
// answer a caller renders includes what each Run has consumed — and a second Run
// shape assembled from a subset of the same columns would be a second answer to
// "what is this Run".
func (s *RunStore) PageRuns(ctx context.Context, sessionID string, statuses []execution.RunStatus, beforeStartedAt int64, beforeRunID string, limit int) ([]transcript.Run, error) {
	query := `SELECT ` + runColumns + `
		 FROM runs AS r
		 LEFT JOIN interrupts AS i ON i.run_id = r.run_id AND i.session_id = r.session_id
		 WHERE r.spawned_by_item_id = ''`
	var args []any
	if sessionID != "" {
		query += ` AND r.session_id = ?`
		args = append(args, sessionID)
	}
	if len(statuses) > 0 {
		columns, err := stateColumns(statuses)
		if err != nil {
			return nil, fmt.Errorf("sqlite: page runs: %w", err)
		}
		query += ` AND r.state IN (` + placeholders(len(columns)) + `)`
		args = append(args, columns...)
	}
	if beforeRunID != "" {
		query += ` AND (r.started_at < ? OR (r.started_at = ? AND r.run_id < ?))`
		args = append(args, beforeStartedAt, beforeStartedAt, beforeRunID)
	}
	query += ` ORDER BY r.started_at DESC, r.run_id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: page runs: %w", err)
	}
	defer rows.Close()

	var out []transcript.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: page runs: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: page runs: %w", err)
	}
	return out, nil
}

// Run returns one Run by id alone, whatever state it is in. The session is not a
// parameter because a run id already identifies exactly one Run: making a caller
// supply the session too would mean it has to know where the Run lives before it
// can ask what the Run is.
func (s *RunStore) Run(ctx context.Context, runID string) (transcript.Run, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 LEFT JOIN interrupts AS i ON i.run_id = r.run_id AND i.session_id = r.session_id
		 WHERE r.run_id = ?`, runID)
	run, err := scanRun(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return transcript.Run{}, false, nil
	case err != nil:
		return transcript.Run{}, false, fmt.Errorf("sqlite: read run %q: %w", runID, err)
	}
	return run, true, nil
}

// stateColumns is the durable encoding of a status filter. An unrecognized status
// is refused rather than skipped: dropping it from the IN list would silently
// widen the page to statuses the caller did not ask for.
func stateColumns(statuses []execution.RunStatus) ([]any, error) {
	out := make([]any, 0, len(statuses))
	for _, status := range statuses {
		if !status.Valid() {
			return nil, fmt.Errorf("unknown run status %d", status)
		}
		out = append(out, stateColumn(status))
	}
	return out, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// runColumns is the whole Run row, joined with the open interrupts a parked Run
// is waiting on — kept in the interrupts table so one park is one record.
const runColumns = `r.run_id, r.session_id, r.spawned_by_item_id, r.state, r.active_segment_id, r.outcome,
	r.provider, r.model, r.detail, r.steps, r.active_duration_ns, r.usage, r.problem,
	r.max_steps, r.max_budget_usd, r.protocol_profile,
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

// coarseState is the column value a Run in state s is stored under. It routes
// through the domain's lifecycle position so a row written by Suspend and a query
// filtering on [execution.StatusWaiting] cannot disagree about which value that
// is — the partial unique index keys on non-terminal, so every terminal RunState
// collapses to the one 'terminal' value (the fine reason lives in runs.outcome).
func coarseState(s execution.RunState) string {
	return stateColumn(s.Status())
}

// stateColumn is the durable spelling of a lifecycle position. It stays an
// explicit table rather than [execution.RunStatus.String]: these three strings are
// on disk and inside the partial unique index's predicate, so a Go rename must not
// be able to rewrite them.
func stateColumn(status execution.RunStatus) string {
	switch status {
	case execution.StatusWaiting:
		return runStateInterrupted
	case execution.StatusFinished:
		return runStateTerminal
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
