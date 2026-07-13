package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
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
	case coarse == runStateRunning:
		return execution.Running, true, nil
	case coarse == runStateInterrupted:
		return execution.Interrupted, true, nil
	default:
		return 0, false, fmt.Errorf("sqlite: read run state: unknown state %q", coarse)
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

// ReconcileOrphans repairs non-terminal Runs abandoned by a process exit before
// any new run is admitted. An Interrupted run with a coherent transcript,
// matching durable interrupt, and resumable process snapshot survives. Every
// other non-terminal row is lost: its running
// transcript items become incomplete, its transcript Run becomes a terminal
// error(run_lost) with a message watermark, and its admission row terminalizes.
// Orphan interrupt rows are removed. The complete cross-table repair commits in
// one transaction, so boot never exposes a half-reconciled lifecycle.
func (s *RunStateStore) ReconcileOrphans(ctx context.Context) (int, error) {
	var reconciled int
	now := time.Now().UTC()
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		active, err := s.nonTerminalRuns(ctx)
		if err != nil {
			return err
		}
		interruptStore := NewInterruptStore(s.db)
		pending, err := interruptStore.List(ctx, "")
		if err != nil {
			return fmt.Errorf("sqlite: reconcile orphan runs: %w", err)
		}
		pendingByRun := make(map[string]interrupts.Pending, len(pending))
		for _, interrupt := range pending {
			pendingByRun[interrupt.RunID] = interrupt
		}

		preserved := make(map[string]struct{}, len(active))
		for _, run := range active {
			pendingInterrupt, hasInterrupt := pendingByRun[run.runID]
			if run.state == execution.Interrupted && hasInterrupt && pendingInterrupt.SessionID == run.sessionID {
				resumable, err := s.validateParkedRun(ctx, run, pendingInterrupt)
				if err != nil {
					return err
				}
				if resumable {
					preserved[run.runID] = struct{}{}
					continue
				}
				if err := s.recoverLostRun(ctx, run, now); err != nil {
					return err
				}
				if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM process_snapshots WHERE id = ?`, pendingInterrupt.ProcessID); err != nil {
					return fmt.Errorf("sqlite: delete unusable process snapshot for run %q: %w", run.runID, err)
				}
				reconciled++
				continue
			}
			if err := s.recoverLostRun(ctx, run, now); err != nil {
				return err
			}
			reconciled++
		}
		for _, interrupt := range pending {
			if _, ok := preserved[interrupt.RunID]; ok {
				continue
			}
			if interrupt.ProcessID != "" {
				if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM process_snapshots WHERE id = ?`, interrupt.ProcessID); err != nil {
					return fmt.Errorf("sqlite: reconcile orphan process snapshot: %w", err)
				}
			}
			if err := interruptStore.Delete(ctx, interrupt.RunID); err != nil {
				return fmt.Errorf("sqlite: reconcile orphan interrupt: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return reconciled, nil
}

// validateParkedRun checks the complete park boundary before boot keeps it
// resumable. A matching row in interrupts is not sufficient: resume also needs
// the interrupted transcript Run, every referenced running item, and a usable
// process snapshot. An impossible partial transcript write means database
// corruption (the transcript park is one transaction), so startup fails loud;
// a missing/unusable process snapshot is an external-resource loss and returns
// resumable=false so reconciliation can terminalize the Run as run_lost.
func (s *RunStateStore) validateParkedRun(ctx context.Context, active nonTerminalRun, pending interrupts.Pending) (bool, error) {
	if pending.RunCreatedAt.IsZero() || pending.CreatedAt.IsZero() || len(pending.Interrupts) == 0 {
		return false, fmt.Errorf("sqlite: validate parked run %q: incomplete interrupt boundary", active.runID)
	}

	items, runs, err := NewTranscriptStore(s.db).List(ctx, active.sessionID)
	if err != nil {
		return false, fmt.Errorf("sqlite: validate parked run %q transcript: %w", active.runID, err)
	}
	var run *transcript.Run
	for i := range runs {
		if runs[i].ID == active.runID {
			run = &runs[i]
			break
		}
	}
	if run == nil {
		return false, fmt.Errorf("sqlite: validate parked run %q: transcript run not found", active.runID)
	}
	if run.State != execution.Interrupted || run.Outcome != nil || run.Result != nil || !run.FinishedAt.IsZero() || run.MessageMark != -1 {
		return false, fmt.Errorf("sqlite: validate parked run %q: invalid interrupted transcript boundary", active.runID)
	}
	if !run.CreatedAt.Equal(pending.RunCreatedAt) {
		return false, fmt.Errorf("sqlite: validate parked run %q: transcript and interrupt creation times differ", active.runID)
	}
	if !reflect.DeepEqual(run.Interrupts, pending.Interrupts) {
		return false, fmt.Errorf("sqlite: validate parked run %q: transcript and pending interrupts differ", active.runID)
	}

	itemsByID := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		itemsByID[item.ID] = item
	}
	seen := make(map[string]struct{}, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
		if interrupt.ItemID == "" {
			return false, fmt.Errorf("sqlite: validate parked run %q: interrupt item id is required", active.runID)
		}
		if _, duplicate := seen[interrupt.ItemID]; duplicate {
			return false, fmt.Errorf("sqlite: validate parked run %q: duplicate interrupt item %q", active.runID, interrupt.ItemID)
		}
		seen[interrupt.ItemID] = struct{}{}
		item, found := itemsByID[interrupt.ItemID]
		if !found || item.RunID != active.runID || item.Status != transcript.ItemRunning {
			return false, fmt.Errorf("sqlite: validate parked run %q: interrupt item %q is not running in the run", active.runID, interrupt.ItemID)
		}
		switch interrupt.Kind {
		case transcript.ApprovalInterrupt:
			if interrupt.Approval == nil || interrupt.Question != nil || item.Kind != transcript.ToolCall || item.Tool == nil {
				return false, fmt.Errorf("sqlite: validate parked run %q: malformed approval item %q", active.runID, interrupt.ItemID)
			}
		case transcript.QuestionInterrupt:
			if interrupt.Question == nil || interrupt.Approval != nil || item.Kind != transcript.QuestionItem || item.Question == nil {
				return false, fmt.Errorf("sqlite: validate parked run %q: malformed question item %q", active.runID, interrupt.ItemID)
			}
		default:
			return false, fmt.Errorf("sqlite: validate parked run %q: unknown interrupt kind %d", active.runID, interrupt.Kind)
		}
	}
	for _, drained := range pending.DrainedTools {
		item, found := itemsByID[drained.ItemID]
		if drained.ItemID == "" || drained.Name == "" || !found || item.RunID != active.runID || item.Kind != transcript.ToolCall || item.Status != transcript.ItemIncomplete {
			return false, fmt.Errorf("sqlite: validate parked run %q: malformed drained tool %q", active.runID, drained.ItemID)
		}
	}
	if pending.ProcessID == "" {
		return false, nil
	}
	return s.hasResumableProcessSnapshot(ctx, pending.ProcessID)
}

func (s *RunStateStore) hasResumableProcessSnapshot(ctx context.Context, processID string) (bool, error) {
	var payload string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT snapshot FROM process_snapshots WHERE id = ?`, processID,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: validate process snapshot %q: %w", processID, err)
	}
	var snapshot core.ProcessSnapshot
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		return false, nil
	}
	if snapshot.ID != processID || snapshot.AgentName == "" {
		return false, nil
	}
	return snapshot.Status == core.StatusWaiting || snapshot.Status == core.StatusPaused, nil
}

type nonTerminalRun struct {
	runID     string
	sessionID string
	state     execution.RunState
}

func (s *RunStateStore) nonTerminalRuns(ctx context.Context) ([]nonTerminalRun, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT run_id, session_id, state FROM runs WHERE state != ? ORDER BY started_at`,
		runStateTerminal)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal runs: %w", err)
	}
	defer rows.Close()

	var out []nonTerminalRun
	for rows.Next() {
		var run nonTerminalRun
		var coarse string
		if err := rows.Scan(&run.runID, &run.sessionID, &coarse); err != nil {
			return nil, fmt.Errorf("sqlite: scan non-terminal run: %w", err)
		}
		switch coarse {
		case runStateRunning:
			run.state = execution.Running
		case runStateInterrupted:
			run.state = execution.Interrupted
		default:
			return nil, fmt.Errorf("sqlite: scan non-terminal run: unknown state %q", coarse)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal runs: %w", err)
	}
	return out, nil
}

func (s *RunStateStore) recoverLostRun(ctx context.Context, active nonTerminalRun, now time.Time) error {
	transcripts := NewTranscriptStore(s.db)
	items, runs, err := transcripts.List(ctx, active.sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: reconcile lost run %q transcript: %w", active.runID, err)
	}
	var run *transcript.Run
	for i := range runs {
		if runs[i].ID == active.runID {
			run = &runs[i]
			break
		}
	}
	if run == nil {
		return fmt.Errorf("sqlite: reconcile lost run %q: transcript run not found", active.runID)
	}
	if run.State != active.state {
		return fmt.Errorf("sqlite: reconcile lost run %q: transcript state %s does not match admission state %s", active.runID, run.State, active.state)
	}

	for _, item := range items {
		if item.RunID != active.runID || item.Status != transcript.ItemRunning {
			continue
		}
		item.Status = transcript.ItemIncomplete
		if item.Kind == transcript.ToolCall {
			item.Error = &transcript.Problem{
				Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem,
				Detail: "tool call interrupted because the run was lost on restart",
			}
		}
		if err := transcripts.AppendItem(ctx, item); err != nil {
			return fmt.Errorf("sqlite: reconcile lost run %q item %q: %w", active.runID, item.ID, err)
		}
	}

	messageMark, err := NewMessageStore(s.db).Count(ctx, active.sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: reconcile lost run %q watermark: %w", active.runID, err)
	}
	next, ok := active.state.RecoverLost()
	if !ok {
		return fmt.Errorf("sqlite: reconcile lost run %q: state %s is not recoverable", active.runID, active.state)
	}
	outcome := execution.OutcomeError
	if run.Result == nil {
		run.Result = &transcript.RunResult{}
	}
	run.State = next
	run.Outcome = new(outcome)
	run.Result.Error = &transcript.Problem{
		Kind: transcript.RunLostProblem, Scope: transcript.RunProblem,
		Detail: "run lost on restart",
	}
	run.Detail = ""
	run.Interrupts = nil
	run.FinishedAt = now
	run.UpdatedAt = now
	run.MessageMark = messageMark
	if err := transcripts.PutRun(ctx, *run); err != nil {
		return fmt.Errorf("sqlite: reconcile lost run %q terminal transcript: %w", active.runID, err)
	}
	if err := s.writeState(ctx, active.sessionID, active.runID, "reconcile lost", active.state, next, outcome.String()); err != nil {
		return err
	}
	return nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure —
// the partial-unique-index rejection that means the session already holds a
// non-terminal run. modernc.org/sqlite surfaces it as a typed *sqlite.Error
// carrying the extended result code.
func isUniqueViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE
}
