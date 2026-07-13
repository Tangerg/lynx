package sessions

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

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
func (c *Coordinator) ApplyRunCancel(ctx context.Context, sessionID, runID string) error {
	return c.s.ApplyCancel(ctx, sessionID, runID)
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
		_ = c.turns.Cancel(ctx, r.ref())
	}
}
