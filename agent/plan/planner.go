package plan

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
)

// PlanOptions carries per-call planner knobs. The most important field is
// ExcludedActions — that's how the runtime asks the planner to ignore a
// recently-replanned action so a misbehaving action doesn't cause an
// infinite loop.
type PlanOptions struct {
	ExcludedActions map[string]struct{}
	MaxIterations   int
}

// Planner is the abstract planner surface. The runtime uses BestValuePlan
// as its main entry point — pick the highest-(value − cost) plan across
// all goals — and falls back to the more granular methods when it needs
// them.
type Planner interface {
	// PlanToGoal targets one specific goal. Returns (nil plan, nil error)
	// when no plan exists (genuinely unreachable); error only on internal
	// failure.
	PlanToGoal(ctx context.Context, start core.WorldState, system *PlanningSystem, goal *core.Goal, options PlanOptions) (*Plan, error)

	// PlansToGoals enumerates plans for every goal in the system, sorted
	// by NetValue descending. Used by debugging and by the embabel-style
	// "let me see all my options" UI.
	PlansToGoals(ctx context.Context, start core.WorldState, system *PlanningSystem, options PlanOptions) ([]*Plan, error)

	// BestValuePlan is the runtime's tick-time entry: pick the single best
	// plan across all goals, honoring the exclusion list.
	BestValuePlan(ctx context.Context, start core.WorldState, system *PlanningSystem, options PlanOptions) (*Plan, error)

	// Prune drops actions that cannot contribute to any goal. Optional —
	// hosts that want unreachable-action elimination call this themselves
	// before deploying an agent.
	Prune(system *PlanningSystem) *PlanningSystem
}
