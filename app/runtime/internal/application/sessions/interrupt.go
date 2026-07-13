package sessions

import (
	"context"
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
}

func (r RunTurnBinding) ref() RunRef {
	return RunRef{SessionID: r.SessionID, TurnID: r.TurnID}
}

// ListOpenInterrupts exposes the run-admission read needed by application/runs.
func (c *Coordinator) ListOpenInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	return c.s.Interrupts().List(ctx, sessionID)
}

// GetOpenInterrupt returns the parked run identified by runID without claiming
// or consuming it. The run use case owns the subsequent admission ordering.
func (c *Coordinator) GetOpenInterrupt(ctx context.Context, runID string) (interrupts.Pending, bool, error) {
	return c.s.Interrupts().Get(ctx, runID)
}

// ApplyRunCancel commits the atomic durable abandon write-set. Executor
// teardown is owned by application/runs for run commands; session rollback and
// deletion continue to use the coordinator's narrow cleanup collaborator.
func (c *Coordinator) ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) error {
	if finishedAt.IsZero() {
		return fmt.Errorf("sessions: cancel parked run %q: finished time is required", runID)
	}
	snapshot, err := c.s.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	idx := slices.IndexFunc(snapshot.Runs, func(run transcript.Run) bool { return run.ID == runID })
	if idx < 0 {
		return fmt.Errorf("sessions: cancel parked run %q: %w", runID, transcript.ErrRunNotFound)
	}
	run := snapshot.Runs[idx]
	state, ok := run.State.Terminate(execution.OutcomeCanceled)
	if !ok || run.State != execution.Interrupted {
		return fmt.Errorf("sessions: cancel parked run %q: state is %s, want interrupted", runID, run.State)
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
			return fmt.Errorf("sessions: cancel parked run %q: interrupt item %q is not running in the run", runID, item.ID)
		}
		item.Status = transcript.ItemIncomplete
		items = append(items, item)
		delete(interruptItems, item.ID)
	}
	if len(interruptItems) != 0 {
		return fmt.Errorf("sessions: cancel parked run %q: transcript is missing an interrupt item", runID)
	}

	outcome := execution.OutcomeCanceled
	run.State = state
	run.Outcome = &outcome
	run.Result = &transcript.RunResult{}
	run.Detail = reason
	run.Interrupts = nil
	run.FinishedAt = finishedAt.UTC()
	run.UpdatedAt = run.FinishedAt
	run.MessageMark = len(snapshot.Messages)
	return c.s.ApplyCancel(ctx, CancelPlan{Run: run, Items: items})
}

func (c *Coordinator) parkedTurns(ctx context.Context, runIDs []string) ([]RunTurnBinding, error) {
	var out []RunTurnBinding
	for _, runID := range runIDs {
		pending, found, err := c.s.Interrupts().Get(ctx, runID)
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
		})
	}
	return out, nil
}

func (c *Coordinator) cancelTurn(ctx context.Context, r RunTurnBinding) {
	if c.turns != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), turnCleanupTimeout)
		defer cancel()
		_ = c.turns.Cancel(cleanupCtx, r.ref())
	}
}
