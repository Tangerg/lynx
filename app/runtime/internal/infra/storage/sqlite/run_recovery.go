package sqlite

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// ProcessSnapshotValidator asks the executor that owns processID whether its
// durable continuation is resumable. false, nil means the external state is
// unusable and the owning Run must be recovered lost; a non-nil error means the
// check itself failed and aborts the reconciliation transaction.
type ProcessSnapshotValidator func(context.Context, string) (bool, error)

// ReconcileOrphans repairs non-terminal Runs abandoned by a process exit before
// any new run is admitted. An Interrupted run with a coherent transcript,
// matching durable interrupt, and resumable process snapshot survives. Every
// other non-terminal row is lost: its running transcript items become
// incomplete, its transcript Run becomes a terminal error(run_lost) with a
// message watermark, and its admission row terminalizes. Orphan interrupt rows
// are removed. The complete cross-table repair commits in one transaction, so
// boot never exposes a half-reconciled lifecycle.
func (s *RunStateStore) ReconcileOrphans(ctx context.Context, validateSnapshot ProcessSnapshotValidator) (int, error) {
	if validateSnapshot == nil {
		return 0, errors.New("sqlite: process snapshot validator is required")
	}
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
		processOwners := make(map[string]string, len(pending))
		for _, interrupt := range pending {
			if interrupt.ProcessID != "" {
				if owner, duplicate := processOwners[interrupt.ProcessID]; duplicate {
					return fmt.Errorf("sqlite: process snapshot %q is owned by interrupts %q and %q", interrupt.ProcessID, owner, interrupt.RunID)
				}
				processOwners[interrupt.ProcessID] = interrupt.RunID
			}
			pendingByRun[interrupt.RunID] = interrupt
		}

		preserved := make(map[string]struct{}, len(active))
		for _, run := range active {
			pendingInterrupt, hasInterrupt := pendingByRun[run.runID]
			if run.state == execution.Interrupted && hasInterrupt && pendingInterrupt.SessionID == run.sessionID {
				resumable, err := s.validateParkedRun(ctx, run, pendingInterrupt, validateSnapshot)
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
				if err := NewProcessStore(s.db).Delete(ctx, pendingInterrupt.ProcessID); err != nil {
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
				if err := NewProcessStore(s.db).Delete(ctx, interrupt.ProcessID); err != nil {
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
func (s *RunStateStore) validateParkedRun(ctx context.Context, active nonTerminalRun, pending interrupts.Pending, validateSnapshot ProcessSnapshotValidator) (bool, error) {
	if pending.RunID != active.runID || pending.SessionID != active.sessionID {
		return false, fmt.Errorf("sqlite: validate parked run %q: interrupt identity is %q/%q, want %q/%q", active.runID, pending.SessionID, pending.RunID, active.sessionID, active.runID)
	}
	// These columns decode via time.Unix(0, ns), so the schema default 0 becomes the
	// 1970 epoch — whose time.IsZero() is false (Go's zero time is year 1). Test the
	// decoded nanos against 0 to actually detect an unset timestamp / incomplete boundary.
	if pending.RunCreatedAt.UnixNano() == 0 || pending.CreatedAt.UnixNano() == 0 || len(pending.Interrupts) == 0 {
		return false, fmt.Errorf("sqlite: validate parked run %q: incomplete interrupt boundary", active.runID)
	}
	if pending.TurnID == "" {
		return false, fmt.Errorf("sqlite: validate parked run %q: turn id is required", active.runID)
	}
	if (pending.Provider == "") != (pending.Model == "") {
		return false, fmt.Errorf("sqlite: validate parked run %q: provider and model must both be set or both be empty", active.runID)
	}
	if active.provider != pending.Provider || active.model != pending.Model {
		return false, fmt.Errorf("sqlite: validate parked run %q: admission model %q/%q differs from interrupt model %q/%q", active.runID, active.provider, active.model, pending.Provider, pending.Model)
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
	if run.Provider != pending.Provider || run.Model != pending.Model {
		return false, fmt.Errorf("sqlite: validate parked run %q: transcript model %q/%q differs from interrupt model %q/%q", active.runID, run.Provider, run.Model, pending.Provider, pending.Model)
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
			if interrupt.Approval == nil || interrupt.Question != nil || item.Kind != transcript.ToolCall || item.Tool == nil ||
				!reflect.DeepEqual(*item.Tool, interrupt.Approval.Tool) {
				return false, fmt.Errorf("sqlite: validate parked run %q: malformed approval item %q", active.runID, interrupt.ItemID)
			}
		case transcript.QuestionInterrupt:
			if interrupt.Question == nil || interrupt.Approval != nil || item.Kind != transcript.QuestionItem || item.Question == nil ||
				!reflect.DeepEqual(item.Question, interrupt.Question) {
				return false, fmt.Errorf("sqlite: validate parked run %q: malformed question item %q", active.runID, interrupt.ItemID)
			}
		default:
			return false, fmt.Errorf("sqlite: validate parked run %q: unknown interrupt kind %d", active.runID, interrupt.Kind)
		}
	}
	for _, item := range items {
		if item.RunID != active.runID || item.Status != transcript.ItemRunning {
			continue
		}
		if _, belongsToInterrupt := seen[item.ID]; !belongsToInterrupt {
			return false, fmt.Errorf("sqlite: validate parked run %q: running item %q has no matching interrupt", active.runID, item.ID)
		}
	}
	drainedSeen := make(map[string]struct{}, len(pending.DrainedTools))
	for _, drained := range pending.DrainedTools {
		item, found := itemsByID[drained.ItemID]
		_, overlapsInterrupt := seen[drained.ItemID]
		_, duplicate := drainedSeen[drained.ItemID]
		if drained.ItemID == "" || drained.Name == "" || duplicate || overlapsInterrupt || !found || item.RunID != active.runID ||
			item.Kind != transcript.ToolCall || item.Status != transcript.ItemIncomplete || item.Tool == nil || item.Tool.Name != drained.Name {
			return false, fmt.Errorf("sqlite: validate parked run %q: malformed drained tool %q", active.runID, drained.ItemID)
		}
		drainedSeen[drained.ItemID] = struct{}{}
	}
	if pending.ProcessID == "" {
		return false, nil
	}
	return s.hasResumableProcessSnapshot(ctx, pending.ProcessID, validateSnapshot)
}

func (s *RunStateStore) hasResumableProcessSnapshot(ctx context.Context, processID string, validateSnapshot ProcessSnapshotValidator) (bool, error) {
	resumable, err := validateSnapshot(ctx, processID)
	if err != nil {
		return false, fmt.Errorf("sqlite: validate process snapshot %q resumable state: %w", processID, err)
	}
	return resumable, nil
}

type nonTerminalRun struct {
	runID     string
	sessionID string
	provider  string
	model     string
	state     execution.RunState
}

func (s *RunStateStore) nonTerminalRuns(ctx context.Context) ([]nonTerminalRun, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT run_id, session_id, provider, model, state FROM runs WHERE state != ? ORDER BY started_at`,
		runStateTerminal)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal runs: %w", err)
	}
	defer rows.Close()

	var out []nonTerminalRun
	for rows.Next() {
		var run nonTerminalRun
		var coarse string
		if err := rows.Scan(&run.runID, &run.sessionID, &run.provider, &run.model, &coarse); err != nil {
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
