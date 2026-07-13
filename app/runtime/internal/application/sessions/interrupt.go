package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// RunTurnBinding binds a protocol run id to the turn that owns its process.
type RunTurnBinding struct {
	RunID     string
	SessionID string
	TurnID    string
}

func (r RunTurnBinding) ref() RunRef {
	return RunRef{SessionID: r.SessionID, TurnID: r.TurnID}
}

// PreparedInterrupt is an open interrupt plus the parked executor handle whose
// continuation can be attached before the decision is delivered.
type PreparedInterrupt struct {
	Pending interrupts.Pending
	Handle  Handle
}

// CancelParkedRun abandons a run that has already left the live run stream and
// is discoverable only through its open interrupt record.
func (c *Coordinator) CancelParkedRun(ctx context.Context, runID string) error {
	pending, found, err := c.s.Interrupts().Get(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		return ErrRunNotFound
	}
	return c.CancelRunBinding(ctx, RunTurnBinding{
		RunID:     runID,
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
	})
}

// CancelRunBinding tears down the bound turn, then drops the open interrupt and
// terminalizes the run's admission row as one atomic write-set (§8.1). The turn
// cancel is best-effort: after a backend restart the durable interrupt may
// outlive the in-memory turn, and abandoning the run still means removing the
// resumable record + freeing the durable admission slot (so the session can
// start a fresh run). For a live cancel the pump also terminalizes, but this
// frees the slot synchronously (and is the ONLY terminalize for a parked cancel,
// whose pump has already exited).
func (c *Coordinator) CancelRunBinding(ctx context.Context, r RunTurnBinding) error {
	c.cancelTurn(ctx, r)
	return c.s.ApplyCancel(ctx, r.SessionID, r.RunID)
}

// PrepareClaimedInterrupt resolves the parked executor handle without consuming
// the interrupt or delivering the decision. If the process-local turn was lost
// across restart, it restores the parked process from the durable snapshot. The
// run coordinator subsequently attaches the event stream and atomically accepts
// the continuation before [Coordinator.ActivatePreparedInterrupt] resumes it.
func (c *Coordinator) PrepareClaimedInterrupt(ctx context.Context, pending interrupts.Pending) (PreparedInterrupt, error) {
	handle, err := c.turns.Prepare(ctx, RunRef{SessionID: pending.SessionID, TurnID: pending.TurnID})
	if err != nil {
		if errors.Is(err, ErrParkClaimed) {
			return PreparedInterrupt{}, ErrInterruptNotOpen
		}
		if !errors.Is(err, ErrTurnNotLive) {
			return PreparedInterrupt{}, err
		}
		handle, err = c.rehydratePendingTurn(ctx, pending)
		if err != nil {
			return PreparedInterrupt{}, errors.Join(ErrRunNotFound, err)
		}
	}
	return PreparedInterrupt{Pending: pending, Handle: handle}, nil
}

// ActivatePreparedInterrupt delivers the user's resolution after the run
// coordinator has attached the continuation stream and committed its durable
// opening. Any executor error is therefore observed by the segment pump and
// becomes a terminal stream event; the consumed interrupt is never resurrected.
func (c *Coordinator) ActivatePreparedInterrupt(ctx context.Context, prepared PreparedInterrupt, resolution interrupts.Resolution, interruptKinds []string) error {
	return c.turns.Resume(ctx, prepared.Handle, resolution, interruptKinds)
}

func (c *Coordinator) rehydratePendingTurn(ctx context.Context, pending interrupts.Pending) (Handle, error) {
	if pending.ProcessID == "" {
		return nil, errors.New("sessions: interrupt has no recorded process id")
	}
	return c.turns.Rehydrate(ctx, RehydrateSpec{
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
		ProcessID: pending.ProcessID,
		Provider:  pending.Provider,
		Model:     pending.Model,
	})
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
