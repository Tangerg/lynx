// Package reactive implements a one-step utility-scoring planner.
//
// Where GOAP searches for an optimal action sequence, the reactive
// planner picks just the *next* action — the one whose effects close
// the most goal preconditions, with low cost as a tie-breaker. The
// resulting [plan.Plan] always has at most one action; the runtime
// drives the agent toward the goal by replanning every tick.
//
// This is the right planner when:
//
//   - the world changes between ticks (event-driven domains where
//     a multi-step plan would be stale by the time the second action
//     runs)
//   - actions are inherently incremental (chat agents, monitoring
//     loops) and the goal is "make progress, then re-evaluate"
//   - the action space is too large for A* but you still want goal-
//     directed behaviour
//
// Mirrors embabel's UtilityPlanner — same shape, different name.
package reactive

import (
	"context"
	"math"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

// plannerTracer is the package-level tracer for the reactive planner.
var plannerTracer = otel.Tracer("lynx/agent/planner")

// Planner is the concrete reactive planner. Stateless across calls;
// safe to share across goroutines.
type Planner struct{}

// NewPlanner returns a reactive planner with default settings. There
// are no knobs today — all per-call options come through
// [plan.PlanOptions].
func NewPlanner() *Planner { return &Planner{} }

// Name is the planner's extension identifier — the value an agent's
// [core.AgentConfig.PlannerName] must match to select this planner.
func (p *Planner) Name() string { return "reactive" }

// PlanToGoal scores each applicable action by how many still-
// unsatisfied goal preconditions its effects would close, picks the
// best one (ties broken by lower cost), and returns it as a one-action
// plan. Actions that would not close any precondition are rejected —
// this guards against the planner repeatedly choosing a "do
// something useless" action whose effects don't move the world toward
// the goal.
//
// Returns:
//   - empty plan when start already satisfies the goal,
//   - one-action plan when an applicable action makes progress,
//   - (nil, nil) when no applicable action makes progress (the runtime
//     interprets this as "stuck" and may drive a stuck-handler).
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	goal *core.Goal,
	options plan.PlanOptions,
) (result *plan.Plan, err error) {
	if err = plan.CheckPlanInputs(start, system, goal); err != nil {
		return nil, err
	}

	_, span := plannerTracer.Start(ctx, "reactive.plan",
		trace.WithAttributes(
			attribute.String("lynx.agent.planner", "reactive"),
			attribute.String("lynx.agent.goal.name", goal.Name),
		),
	)
	defer func() {
		if result != nil {
			span.SetAttributes(attribute.Int("lynx.agent.plan.length", len(result.Actions)))
		}
		span.End()
	}()

	if goal.IsSatisfiedBy(start) {
		result = &plan.Plan{Goal: goal}
		return result, nil
	}
	best := p.bestApplicable(start, system.Actions, goal, options.ExcludedActions)
	if best == nil {
		return nil, nil
	}
	result = &plan.Plan{Actions: []core.Action{best}, Goal: goal}
	return result, nil
}

// bestApplicable picks the action whose effects close the most
// still-unsatisfied goal preconditions; ties broken by lower cost.
// Actions whose progress score is 0 (would not close any
// precondition) are rejected — the planner returns nil rather than
// picking a "do something useless" action.
//
// Cost policy: actions with a nil [core.ActionMetadata.Cost] score at
// +Inf, so any cost-attached competitor with equal progress wins the
// tie. Use [core.Static](v) to attach a constant cost; the canonical
// [core.NewAction] constructor fills in [core.Static](1.0) when none
// is supplied.
func (p *Planner) bestApplicable(
	start core.WorldState,
	actions []core.Action,
	goal *core.Goal,
	excluded map[string]struct{},
) core.Action {
	state := start.State()
	unsat := unsatisfiedPreconditions(state, goal)

	var best core.Action
	bestProgress := 0
	bestCost := math.Inf(1)

	for _, action := range actions {
		if action == nil {
			continue
		}
		meta := action.Metadata()
		if _, skip := excluded[meta.Name]; skip {
			continue
		}
		if !meta.IsApplicableIn(state) {
			continue
		}

		progress := progressTowardsGoal(meta.Effects, unsat)
		if progress == 0 {
			continue
		}

		cost := math.Inf(1)
		if meta.Cost != nil {
			cost = meta.Cost(start)
		}

		if progress > bestProgress || (progress == bestProgress && cost < bestCost) {
			best = action
			bestProgress = progress
			bestCost = cost
		}
	}
	return best
}

// progressTowardsGoal counts how many still-unsatisfied goal
// preconditions this effect map would establish.
func progressTowardsGoal(effects core.EffectSpec, unsatisfied map[string]core.Determination) int {
	progress := 0
	for key, required := range unsatisfied {
		if effects[key] == required {
			progress++
		}
	}
	return progress
}

// unsatisfiedPreconditions returns the subset of goal preconditions
// not yet matched by state.
func unsatisfiedPreconditions(state map[string]core.Determination, goal *core.Goal) map[string]core.Determination {
	out := make(map[string]core.Determination, len(goal.Preconditions()))
	for key, required := range goal.Preconditions() {
		if state[key] != required {
			out[key] = required
		}
	}
	return out
}
