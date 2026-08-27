package sessions

import (
	"context"
	"errors"

	planapp "github.com/Tangerg/scope/app/runtime/internal/application/plans"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
)

// PlanServices is the complete optional Plan capability. Grouping the two
// collaborators prevents a runtime where boundary history is enabled but the
// corresponding aggregate transition cannot be committed.
type PlanServices struct {
	Boundaries   PlanBoundaries
	Replacements PlanReplacements
}

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
	if c.plan == nil {
		return PlanBoundary{}, nil
	}
	if runID == "" {
		return PlanBoundary{Recorded: true}, nil
	}
	steps, recorded, err := c.plan.Boundaries.Boundary(ctx, runID)
	if err != nil {
		return PlanBoundary{}, err
	}
	return PlanBoundary{Steps: steps, Recorded: recorded}, nil
}

func (c *Coordinator) prepareBoundaryPlanReplacement(
	ctx context.Context,
	sessionID string,
	boundary PlanBoundary,
) (*planapp.Replacement, error) {
	if !boundary.Recorded {
		return nil, nil
	}
	replacement, err := c.plan.Replacements.PrepareReplacement(ctx, sessionID, boundary.Steps)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}

func (c *Coordinator) prepareInitialPlanReplacement(steps []plan.Step) (*planapp.Replacement, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	if c.plan == nil {
		return nil, errors.New("sessions: cannot seed a Plan when Plan support is disabled")
	}
	replacement, err := c.plan.Replacements.PrepareInitial(steps)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}

func (c *Coordinator) prepareRestoredPlanReplacement(
	ctx context.Context,
	sessionID string,
	steps []plan.Step,
) (*planapp.Replacement, error) {
	if c.plan == nil {
		if len(steps) > 0 {
			return nil, errors.New("sessions: cannot restore a Plan when Plan support is disabled")
		}
		return nil, nil
	}
	replacement, err := c.plan.Replacements.PrepareReplacement(ctx, sessionID, steps)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}
