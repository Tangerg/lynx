package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

const turnCleanupTimeout = 5 * time.Second

// RunTurnBinding identifies a parked turn that a session mutation must tear
// down after its durable write-set commits.
type RunTurnBinding struct {
	RunID            string
	SessionID        string
	TurnID           string
	CheckpointRootID string
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

// ApplyRunLost atomically ends a parked run whose executor checkpoint cannot be
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
	if err := pending.Validate(); err != nil {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: invalid Pending: %w", runID, err)
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return transcript.Run{}, err
	}
	runsByID := make(map[string]transcript.Run, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		if _, duplicate := runsByID[run.ID]; duplicate {
			return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: duplicate Run %q", runID, run.ID)
		}
		runsByID[run.ID] = run
	}
	rootAdmission, rootFound := runsByID[runID]
	if !rootFound || !rootAdmission.Lineage().IsRoot() {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: root Run is missing", runID)
	}
	if !pending.Capabilities.Equal(rootAdmission.Capabilities) {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: Pending run capabilities differ from root Run admission", runID)
	}
	targetRunIDs := make(map[string]struct{}, len(pending.Continuations))
	for _, continuation := range pending.Continuations {
		targetRunIDs[continuation.RunID] = struct{}{}
	}
	for _, run := range snapshot.Runs {
		if run.Lineage().TreeRootID(run.ID) != runID || run.State.IsTerminal() {
			continue
		}
		if _, targeted := targetRunIDs[run.ID]; !targeted {
			return transcript.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: non-terminal Run %q has no Pending continuation",
				runID,
				run.ID,
			)
		}
	}

	terminalRuns := make([]transcript.Run, 0, len(pending.Continuations))
	for _, continuation := range pending.Continuations {
		run, found := runsByID[continuation.RunID]
		if !found {
			return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: Run %q is missing", runID, continuation.RunID)
		}
		if run.SessionID != sessionID || run.State != execution.Interrupted ||
			run.Lineage() != continuation.Lineage || run.ModelSelection != continuation.ModelSelection ||
			!run.CreatedAt.Equal(continuation.RunCreatedAt) ||
			!run.Metrics.Equal(continuation.Metrics) || run.Limits != continuation.Limits ||
			!run.Capabilities.Equal(rootAdmission.Capabilities) {
			return transcript.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q differs from its continuation",
				runID,
				run.ID,
			)
		}
		var (
			state execution.RunState
			ok    bool
		)
		switch outcome {
		case execution.OutcomeCanceled:
			state, ok = run.State.Terminate(outcome)
		case execution.OutcomeError:
			state, ok = run.State.RecoverLost()
		default:
			return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: unsupported outcome %s", runID, outcome)
		}
		if !ok {
			return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run %q: cannot apply outcome %s", run.ID, outcome)
		}
		run.State = state
		run.Outcome = &outcome
		if outcome == execution.OutcomeError {
			run.Error = &transcript.Problem{
				Kind:   transcript.RunLostProblem,
				Scope:  transcript.RunProblem,
				Detail: "the parked Run tree's executor checkpoint could not be restored",
			}
		}
		run.Detail = detail
		run.Interrupts = nil
		run.FinishedAt = finishedAt.UTC()
		run.UpdatedAt = run.FinishedAt
		run.MessageMark = len(snapshot.Messages)
		terminalRuns = append(terminalRuns, run)
	}
	rootRun := terminalRuns[len(terminalRuns)-1]
	if rootRun.ID != runID || rootRun.GoalLeaseID != pending.GoalLeaseID {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: root admission differs from Pending", runID)
	}

	interruptItems := make(map[string]string, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
		interruptItems[interrupt.ItemID] = interrupt.RunID
	}
	items := make([]transcript.Item, 0, len(interruptItems))
	for _, item := range snapshot.Items {
		ownerRunID, found := interruptItems[item.ID]
		if !found {
			if _, targeted := targetRunIDs[item.RunID]; targeted && item.Status == transcript.ItemRunning {
				return transcript.Run{}, fmt.Errorf(
					"sessions: terminalize parked Run tree %q: Running Item %q has no matching interrupt",
					runID,
					item.ID,
				)
			}
			continue
		}
		if item.SessionID != sessionID || item.RunID != ownerRunID || item.Status != transcript.ItemRunning {
			return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: interrupt Item %q is not Running in Run %q", runID, item.ID, ownerRunID)
		}
		item.Status = transcript.ItemIncomplete
		if item.Kind == transcript.ToolCall {
			item.FinishedAt = finishedAt.UTC()
			if outcome == execution.OutcomeError {
				item.Error = &transcript.Problem{
					Kind:  transcript.ToolFailedProblem,
					Scope: transcript.ToolProblem,
					// Distinct from a tool that ran and failed, and from one cut off by a
					// restart: this call was still awaiting its approval or answer when the
					// run it belonged to was declared unresumable.
					Detail: "tool call abandoned because its run could not be resumed",
				}
			}
		}
		items = append(items, item)
		delete(interruptItems, item.ID)
	}
	if len(interruptItems) != 0 {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: transcript is missing an interrupt Item", runID)
	}
	root, ok := pending.RootContinuation()
	if !ok {
		return transcript.Run{}, fmt.Errorf("sessions: terminalize parked Run tree %q: root continuation is missing", runID)
	}
	plan := TerminalPlan{Runs: terminalRuns, Items: items, CheckpointRootID: root.ProcessID}
	if rootRun.GoalLeaseID != "" {
		turn := goal.TurnRecord{
			SessionID:   rootRun.SessionID,
			LeaseID:     rootRun.GoalLeaseID,
			RunID:       rootRun.ID,
			Outcome:     *rootRun.Outcome,
			Steps:       rootRun.Metrics.Steps,
			CompletedAt: rootRun.FinishedAt,
		}
		if rootRun.Metrics.Usage != nil && rootRun.Metrics.Usage.CostUSD != nil {
			turn.CostUSD = *rootRun.Metrics.Usage.CostUSD
		}
		plan.GoalTurn = &turn
	}
	if err := plan.Validate(); err != nil {
		return transcript.Run{}, err
	}
	if err := c.writes.ApplyTerminal(ctx, plan); err != nil {
		return transcript.Run{}, err
	}
	// One write-set ended the run and dropped the set it was parked on, so one place
	// reports both — for a cancel and for a park declared unresumable alike. The run
	// command layer deliberately does not publish this again: it would be a second
	// author for the same commit, and only one of them would ever be updated.
	runIDs := make([]string, len(terminalRuns))
	for index, run := range terminalRuns {
		runIDs[index] = run.ID
	}
	notices := []change.Notice{
		change.InSession(change.Runs, sessionID, runIDs...),
		change.InSession(change.Interrupts, sessionID, runID),
		change.InSession(change.Sessions, sessionID),
	}
	if plan.GoalTurn != nil {
		notices = append(notices, change.InSession(change.Goals, sessionID))
	}
	c.changed.Notify(notices...)
	return rootRun, nil
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
			RunID:            pending.RootRunID,
			SessionID:        pending.SessionID,
			TurnID:           pending.TurnID,
			CheckpointRootID: root.ProcessID,
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
