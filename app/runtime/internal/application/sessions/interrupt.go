package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

const turnCleanupTimeout = 5 * time.Second

// RunTurnBinding identifies a parked turn that a session mutation must tear
// down after its durable write-set commits.
type RunTurnBinding struct {
	RunID     string
	SessionID string
	TurnID    string
	ProcessID string
}

func (r RunTurnBinding) ref() RunRef {
	return RunRef{SessionID: r.SessionID, TurnID: r.TurnID}
}

// ListOpenInterrupts exposes the run-admission read needed by application/runs.
func (c *Coordinator) ListOpenInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	if c.interrupts == nil {
		return nil, errors.New("sessions: interrupt store is unavailable")
	}
	return c.interrupts.List(ctx, sessionID)
}

// GetOpenInterrupt returns the parked run identified by runID without claiming
// or consuming it. The run use case owns the subsequent admission ordering.
func (c *Coordinator) GetOpenInterrupt(ctx context.Context, runID string) (interrupts.Pending, bool, error) {
	if c.interrupts == nil {
		return interrupts.Pending{}, false, errors.New("sessions: interrupt store is unavailable")
	}
	return c.interrupts.Get(ctx, runID)
}

// ApplyRunCancel commits the atomic durable abandon write-set. Executor
// teardown is owned by application/runs for run commands; session rollback and
// deletion continue to use the coordinator's narrow cleanup collaborator.
func (c *Coordinator) ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) error {
	return c.terminalizeParkedRun(ctx, sessionID, runID, finishedAt, execution.OutcomeCanceled, reason)
}

// ApplyRunLost atomically ends a parked run whose process snapshot cannot be
// restored. It uses the recovery transition because the interrupted Run never
// resumed into a normal executor terminal path.
func (c *Coordinator) ApplyRunLost(ctx context.Context, sessionID, runID string, finishedAt time.Time) error {
	return c.terminalizeParkedRun(ctx, sessionID, runID, finishedAt, execution.OutcomeError, "")
}

func (c *Coordinator) terminalizeParkedRun(ctx context.Context, sessionID, runID string, finishedAt time.Time, outcome execution.Outcome, detail string) error {
	if finishedAt.IsZero() {
		return fmt.Errorf("sessions: terminalize parked run %q: finished time is required", runID)
	}
	if c.interrupts == nil || c.snapshots == nil || c.writes == nil {
		return errors.New("sessions: interrupt lifecycle persistence is unavailable")
	}
	pending, found, err := c.interrupts.Get(ctx, runID)
	if err != nil {
		return err
	}
	if !found || pending.SessionID != sessionID {
		return fmt.Errorf("sessions: terminalize parked run %q: open interrupt not found for session %q", runID, sessionID)
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	idx := slices.IndexFunc(snapshot.Runs, func(run transcript.Run) bool { return run.ID == runID })
	if idx < 0 {
		return fmt.Errorf("sessions: terminalize parked run %q: %w", runID, transcript.ErrRunNotFound)
	}
	run := snapshot.Runs[idx]
	if run.State != execution.Interrupted {
		return fmt.Errorf("sessions: terminalize parked run %q: state is %s, want interrupted", runID, run.State)
	}
	var state execution.RunState
	var ok bool
	switch outcome {
	case execution.OutcomeCanceled:
		state, ok = run.State.Terminate(outcome)
	case execution.OutcomeError:
		state, ok = run.State.RecoverLost()
	default:
		return fmt.Errorf("sessions: terminalize parked run %q: unsupported outcome %s", runID, outcome)
	}
	if !ok {
		return fmt.Errorf("sessions: terminalize parked run %q: cannot apply outcome %s", runID, outcome)
	}

	interruptItems := make(map[string]struct{}, len(run.Interrupts))
	for _, interrupt := range run.Interrupts {
		interruptItems[interrupt.ItemID] = struct{}{}
	}
	items := make([]transcript.Item, 0, len(interruptItems))
	for _, item := range snapshot.Items {
		if _, found := interruptItems[item.ID]; !found {
			continue
		}
		if item.RunID != runID || item.Status != transcript.ItemRunning {
			return fmt.Errorf("sessions: terminalize parked run %q: interrupt item %q is not running in the run", runID, item.ID)
		}
		item.Status = transcript.ItemIncomplete
		if outcome == execution.OutcomeError && item.Kind == transcript.ToolCall {
			item.Error = &transcript.Problem{
				Kind:   transcript.ToolFailedProblem,
				Scope:  transcript.ToolProblem,
				Detail: "tool call interrupted because the run process state was lost",
			}
		}
		items = append(items, item)
		delete(interruptItems, item.ID)
	}
	if len(interruptItems) != 0 {
		return fmt.Errorf("sessions: terminalize parked run %q: transcript is missing an interrupt item", runID)
	}

	run.State = state
	run.Outcome = &outcome
	run.Result = &transcript.RunResult{}
	if outcome == execution.OutcomeError {
		run.Result.Error = &transcript.Problem{
			Kind:   transcript.RunLostProblem,
			Scope:  transcript.RunProblem,
			Detail: "run process state is unavailable",
		}
	}
	run.Detail = detail
	run.Interrupts = nil
	run.FinishedAt = finishedAt.UTC()
	run.UpdatedAt = run.FinishedAt
	run.MessageMark = len(snapshot.Messages)
	return c.writes.ApplyTerminal(ctx, TerminalPlan{Run: run, Items: items, ProcessID: pending.ProcessID})
}

func (c *Coordinator) parkedTurns(ctx context.Context, runIDs []string) ([]RunTurnBinding, error) {
	var out []RunTurnBinding
	for _, runID := range runIDs {
		if c.interrupts == nil {
			return nil, errors.New("sessions: interrupt store is unavailable")
		}
		pending, found, err := c.interrupts.Get(ctx, runID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		out = append(out, RunTurnBinding{
			RunID:     pending.RunID,
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
			ProcessID: pending.ProcessID,
		})
	}
	return out, nil
}

func (c *Coordinator) cancelTurn(ctx context.Context, r RunTurnBinding) error {
	if c.turns == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), turnCleanupTimeout)
	defer cancel()
	if err := c.turns.Cancel(cleanupCtx, r.ref()); err != nil {
		return fmt.Errorf("sessions: cancel turn %q for run %q: %w", r.TurnID, r.RunID, err)
	}
	return nil
}
