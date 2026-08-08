package runs

import (
	"context"
	"errors"
	"fmt"
)

// Steer addresses the Segment the command names through execution control.
//
// It resolves through the same authority a subscribe does
// ([Coordinator.addressLiveSegment]), so "this run is waiting" or "that segment
// has been replaced" is one answer with one spelling rather than two entry
// points each guessing from the live registry.
func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) error {
	live, err := c.addressLiveSegment(ctx, cmd.RunID, cmd.ExpectedSegmentID)
	if err != nil {
		return err
	}
	if c.steering == nil {
		return errors.New("runs: execution steerer is required")
	}
	rec := live.record
	if err := c.steering.Steer(ctx, ExecutorRef{SessionID: rec.SessionID, ExecutorID: rec.ExecutorID}, cmd.Input); err != nil {
		if errors.Is(err, ErrExecutorNotLive) {
			// Execution ended between resolving the record and delivering: the Run is
			// finishing, which is the same thing the durable record would say a moment
			// from now.
			return fmt.Errorf("%w: %w", ErrRunFinished, err)
		}
		return err
	}
	return nil
}
