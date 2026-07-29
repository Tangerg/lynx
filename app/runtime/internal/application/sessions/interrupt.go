package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
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

func (r RunTurnBinding) ref() execution.TurnRef {
	return execution.TurnRef{SessionID: r.SessionID, TurnID: r.TurnID}
}

// ListOpenInterrupts exposes the run-admission read needed by application/runs.
func (c *Coordinator) ListOpenInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	if c.interrupts == nil {
		return nil, errors.New("sessions: interrupt store is unavailable")
	}
	return c.interrupts.List(ctx, sessionID)
}

// ActiveRun returns the Session's non-terminal Run — the one the admission
// invariant allows at most one of — reporting false when the Session holds none.
//
// It reads the durable record rather than a live registry: a Run parked on a person,
// or admitted before a restart, is just as much "the Run this Session already has",
// and the registry knows neither.
func (c *Coordinator) ActiveRun(ctx context.Context, sessionID string) (transcript.Run, bool, error) {
	runs, err := c.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return transcript.Run{}, false, err
	}
	for _, run := range runs {
		if !run.State.IsTerminal() {
			return run, true, nil
		}
	}
	return transcript.Run{}, false, nil
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
func (c *Coordinator) ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) (transcript.Run, error) {
	return c.terminalizeParkedRun(ctx, sessionID, runID, finishedAt, execution.OutcomeCanceled, reason)
}

// ApplyRunLost atomically ends a parked run whose process snapshot cannot be
// restored. It uses the recovery transition because the interrupted Run never
// resumed into a normal executor terminal path.
func (c *Coordinator) ApplyRunLost(ctx context.Context, sessionID, runID string, finishedAt time.Time) error {
	_, err := c.terminalizeParkedRun(ctx, sessionID, runID, finishedAt, execution.OutcomeError, "")
	return err
}

func (c *Coordinator) terminalizeParkedRun(ctx context.Context, sessionID, runID string, finishedAt time.Time, outcome execution.Outcome, detail string) (transcript.Run, error) {
	if finishedAt.IsZero() {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: finished time is required", runID)
	}
	if c.interrupts == nil || c.snapshots == nil || c.writes == nil {
		return transcript.Run{}, errors.New("sessions: interrupt lifecycle persistence is unavailable")
	}
	pending, found, err := c.interrupts.Get(ctx, runID)
	if err != nil {
		return transcript.Run{}, err
	}
	if !found || pending.SessionID != sessionID {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: open interrupt not found for session %q", runID, sessionID)
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return transcript.Run{}, err
	}
	idx := slices.IndexFunc(snapshot.Runs, func(run transcript.Run) bool { return run.ID == runID })
	if idx < 0 {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: %w", runID, transcript.ErrRunNotFound)
	}
	run := snapshot.Runs[idx]
	if run.State != execution.Interrupted {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: state is %s, want interrupted", runID, run.State)
	}
	var state execution.RunState
	var ok bool
	switch outcome {
	case execution.OutcomeCanceled:
		state, ok = run.State.Terminate(outcome)
	case execution.OutcomeError:
		state, ok = run.State.RecoverLost()
	default:
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: unsupported outcome %s", runID, outcome)
	}
	if !ok {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: cannot apply outcome %s", runID, outcome)
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
			return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: interrupt item %q is not running in the run", runID, item.ID)
		}
		item.Status = transcript.ItemIncomplete
		if outcome == execution.OutcomeError && item.Kind == transcript.ToolCall {
			item.Error = &transcript.Problem{
				Kind:  transcript.ToolFailedProblem,
				Scope: transcript.ToolProblem,
				// Distinct from a tool that ran and failed, and from one cut off by a
				// restart: this call was still awaiting its approval or answer when the
				// run it belonged to was declared unresumable.
				Detail: "tool call abandoned because its run could not be resumed",
			}
		}
		items = append(items, item)
		delete(interruptItems, item.ID)
	}
	if len(interruptItems) != 0 {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: transcript is missing an interrupt item", runID)
	}

	run.State = state
	run.Outcome = &outcome
	if outcome == execution.OutcomeError {
		run.Error = &transcript.Problem{
			Kind:  transcript.RunLostProblem,
			Scope: transcript.RunProblem,
			// This run was parked on an interrupt and its process snapshot turned out
			// to be unrestorable — it never re-entered the executor, so no terminal
			// path could describe it.
			Detail: "the parked run's process snapshot could not be restored",
		}
	}
	run.Detail = detail
	run.Interrupts = nil
	run.FinishedAt = finishedAt.UTC()
	run.UpdatedAt = run.FinishedAt
	run.MessageMark = len(snapshot.Messages)
	root, ok := pending.RootContinuation()
	if !ok {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked run %q: root continuation is missing", runID)
	}
	if err := c.writes.ApplyTerminal(ctx, TerminalPlan{Run: run, Items: items, ProcessID: root.ProcessID}); err != nil {
		return transcript.Run{}, err
	}
	// One write-set ended the run and dropped the set it was parked on, so one place
	// reports both — for a cancel and for a park declared unresumable alike. The run
	// command layer deliberately does not publish this again: it would be a second
	// author for the same commit, and only one of them would ever be updated.
	c.changed.Notify(
		change.InSession(change.Runs, sessionID, runID),
		change.InSession(change.Interrupts, sessionID, runID),
		change.InSession(change.Sessions, sessionID),
	)
	return run, nil
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
		root, ok := pending.RootContinuation()
		if !ok {
			return nil, fmt.Errorf("sessions: parked run %q has no root continuation", pending.RootRunID)
		}
		out = append(out, RunTurnBinding{
			RunID:     pending.RootRunID,
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
			ProcessID: root.ProcessID,
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
