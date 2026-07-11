package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// ResumedInterrupt is the claimed interrupt plus the opaque handle its
// continuation should stream from.
type ResumedInterrupt struct {
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

// ResumeClaimedInterrupt consumes an open interrupt and resumes its parked
// turn. If the live turn disappeared after a backend restart, it rebuilds the
// process from the durable interrupt snapshot before returning the handle.
func (c *Coordinator) ResumeClaimedInterrupt(ctx context.Context, parentRunID string, resolution interrupts.Resolution, interruptKinds []string) (ResumedInterrupt, error) {
	pending, ok, err := c.s.Interrupts().Consume(ctx, parentRunID)
	if err != nil {
		return ResumedInterrupt{}, err
	}
	if !ok {
		return ResumedInterrupt{}, ErrInterruptNotOpen
	}

	handle, err := c.turns.Resume(ctx, RunRef{SessionID: pending.SessionID, TurnID: pending.TurnID}, resolution, interruptKinds)
	if err != nil {
		if errors.Is(err, ErrParkClaimed) {
			return ResumedInterrupt{}, ErrInterruptNotOpen
		}
		if !errors.Is(err, ErrTurnNotLive) {
			return ResumedInterrupt{}, err
		}
		handle, err = c.rehydratePendingTurn(ctx, pending, resolution.Approved, interruptKinds)
		if err != nil {
			// Rehydrate errors before the decision reaches a restored process are
			// uncommitted: put the claim back so a transient resolver/storage failure
			// does not silently destroy the user's open interrupt. Once the turn layer
			// marks the failure committed it has already terminalized the process, and
			// restoring would create a ghost resumable record.
			if !errors.Is(err, ErrRehydrateCommitted) {
				return ResumedInterrupt{}, errors.Join(ErrRunNotFound, c.restoreConsumedInterrupt(ctx, pending))
			}
			return ResumedInterrupt{}, ErrRunNotFound
		}
	}

	return ResumedInterrupt{Pending: pending, Handle: handle}, nil
}

const interruptCompensationTimeout = 2 * time.Second

func (c *Coordinator) restoreConsumedInterrupt(ctx context.Context, pending interrupts.Pending) error {
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), interruptCompensationTimeout)
	defer cancel()
	if err := c.s.Interrupts().Put(restoreCtx, pending); err != nil {
		return fmt.Errorf("sessions: restore consumed interrupt: %w", err)
	}
	return nil
}

func (c *Coordinator) rehydratePendingTurn(ctx context.Context, pending interrupts.Pending, approved bool, interruptKinds []string) (Handle, error) {
	if pending.ProcessID == "" {
		return nil, errors.New("sessions: interrupt has no recorded process id")
	}
	return c.turns.Rehydrate(ctx, RehydrateSpec{
		SessionID:      pending.SessionID,
		ProcessID:      pending.ProcessID,
		Approved:       approved,
		Provider:       pending.Provider,
		Model:          pending.Model,
		InterruptKinds: interruptKinds,
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
			RunID:     pending.ParentRunID,
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
