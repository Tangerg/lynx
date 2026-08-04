package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// Steer addresses the segment the command names and lets the turn adapter
// recover the concrete executor handle.
//
// It resolves through the same authority a subscribe does
// ([Coordinator.addressLiveSegment]), so "this run is waiting" or "that segment
// has been replaced" is one answer with one spelling rather than two entry
// points each guessing from the live registry.
func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) error {
	if c.turns == nil {
		return errors.New("runs: turn control is required")
	}
	live, err := c.addressLiveSegment(ctx, cmd.RunID, cmd.ExpectedSegmentID)
	if err != nil {
		return err
	}
	rec := live.record
	if err := c.turns.Steer(ctx, execution.TurnRef{SessionID: rec.SessionID, TurnID: rec.TurnID}, cmd.Input); err != nil {
		if errors.Is(err, ErrTurnNotLive) {
			// The turn ended between resolving the record and delivering: the run is
			// finishing, which is the same thing the durable record would say a moment
			// from now.
			return fmt.Errorf("%w: %w", ErrRunFinished, err)
		}
		return err
	}
	return nil
}
