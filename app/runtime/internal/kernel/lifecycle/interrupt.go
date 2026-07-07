package lifecycle

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// TurnCanceler is the turn dispatcher slice needed to abandon a run.
type TurnCanceler interface {
	Cancel(context.Context, turn.TurnHandle) error
}

// TurnResumer is the turn dispatcher slice needed to continue an interrupt.
type TurnResumer interface {
	Resume(context.Context, turn.TurnHandle, interrupts.Resolution) error
	Rehydrate(context.Context, turn.RehydrateRequest) (turn.TurnHandle, error)
}

// RunTurn binds a protocol run id to the turn handle that owns its process.
type RunTurn struct {
	RunID     string
	SessionID string
	TurnID    string
}

// ResumedInterrupt is the claimed interrupt plus the turn handle its
// continuation should stream from.
type ResumedInterrupt struct {
	Pending interrupts.Pending
	Handle  turn.TurnHandle
}

// CancelParkedRun abandons a run that has already left the live run stream and
// is discoverable only through its open interrupt record.
func (c *Coordinator) CancelParkedRun(ctx context.Context, turns TurnCanceler, runID string) error {
	pending, found, err := c.s.Interrupts().Get(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		return ErrRunNotFound
	}
	return c.CancelRunTurn(ctx, turns, RunTurn{
		RunID:     runID,
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
	})
}

// CancelRunTurn tears down the turn before dropping the durable interrupt
// record. The turn cancel is best-effort: after a backend restart the durable
// interrupt may outlive the in-memory turn, and abandoning the run still means
// removing the resumable record.
func (c *Coordinator) CancelRunTurn(ctx context.Context, turns TurnCanceler, r RunTurn) error {
	c.cancelTurn(ctx, turns, r)
	return c.s.Interrupts().Delete(ctx, r.RunID)
}

// ResumeClaimedInterrupt consumes an open interrupt and resumes its parked
// turn. If the live turn disappeared after a backend restart, it rebuilds the
// process from the durable interrupt snapshot before returning the handle.
func (c *Coordinator) ResumeClaimedInterrupt(ctx context.Context, turns TurnResumer, parentRunID string, resolution interrupts.Resolution) (ResumedInterrupt, error) {
	pending, ok, err := c.s.Interrupts().Consume(ctx, parentRunID)
	if err != nil {
		return ResumedInterrupt{}, err
	}
	if !ok {
		return ResumedInterrupt{}, ErrInterruptNotOpen
	}

	handle := turn.TurnHandle{SessionID: pending.SessionID, TurnID: pending.TurnID}
	if err := turns.Resume(ctx, handle, resolution); err != nil {
		if errors.Is(err, turn.ErrParkClaimed) {
			return ResumedInterrupt{}, ErrInterruptNotOpen
		}
		if !errors.Is(err, turn.ErrTurnNotFound) {
			return ResumedInterrupt{}, err
		}
		handle, err = rehydratePendingTurn(ctx, turns, pending, resolution.Approved)
		if err != nil {
			return ResumedInterrupt{}, ErrRunNotFound
		}
	}

	return ResumedInterrupt{Pending: pending, Handle: handle}, nil
}

func rehydratePendingTurn(ctx context.Context, turns TurnResumer, pending interrupts.Pending, approved bool) (turn.TurnHandle, error) {
	if pending.ProcessID == "" {
		return turn.TurnHandle{}, errors.New("lifecycle: interrupt has no recorded process id")
	}
	return turns.Rehydrate(ctx, turn.RehydrateRequest{
		SessionID: pending.SessionID,
		ProcessID: pending.ProcessID,
		Approved:  approved,
		Provider:  pending.Provider,
		Model:     pending.Model,
	})
}

func (c *Coordinator) cancelParkedInterrupts(ctx context.Context, turns TurnCanceler, sessionID string) {
	pending, err := c.s.Interrupts().List(ctx, sessionID)
	if err != nil {
		return
	}
	for _, p := range pending {
		_ = c.CancelRunTurn(ctx, turns, RunTurn{
			RunID:     p.ParentRunID,
			SessionID: p.SessionID,
			TurnID:    p.TurnID,
		})
	}
}

func (c *Coordinator) parkedTurns(ctx context.Context, runIDs []string) ([]RunTurn, error) {
	out := make([]RunTurn, 0)
	for _, runID := range runIDs {
		pending, found, err := c.s.Interrupts().Get(ctx, runID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		out = append(out, RunTurn{
			RunID:     pending.ParentRunID,
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
		})
	}
	return out, nil
}

func (c *Coordinator) cancelTurn(ctx context.Context, turns TurnCanceler, r RunTurn) {
	if turns != nil {
		_ = turns.Cancel(ctx, turn.TurnHandle{SessionID: r.SessionID, TurnID: r.TurnID})
	}
}
