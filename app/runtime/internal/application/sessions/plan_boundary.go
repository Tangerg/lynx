package sessions

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

// PlanBoundary is a Plan recovered from a Run boundary: the value, and
// whether that boundary recorded one at all. The difference is not cosmetic — an
// unrecorded boundary must leave the live list untouched, while a recorded empty
// one must clear it, and a single nil slice cannot say which is meant.
type PlanBoundary struct {
	Steps    []plan.Step
	Recorded bool
}

// planBoundary resolves the Plan the boundary at runID held. An empty runID
// is a boundary that keeps no run at all: it predates every list this session ever
// wrote, so its value is the empty list — known, not unknown. Otherwise the answer
// is whatever that run recorded when it ended, including "nothing was recorded",
// which the caller must not turn into emptiness (an imported run's boundaries were
// never captured; see [PlanBoundaries]).
func (c *Coordinator) planBoundary(ctx context.Context, runID string) (PlanBoundary, error) {
	if runID == "" {
		return PlanBoundary{Recorded: true}, nil
	}
	if c.boundaries == nil {
		return PlanBoundary{}, nil
	}
	steps, recorded, err := c.boundaries.Boundary(ctx, runID)
	if err != nil {
		return PlanBoundary{}, err
	}
	return PlanBoundary{Steps: steps, Recorded: recorded}, nil
}
