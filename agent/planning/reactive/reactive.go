package reactive

import (
	"context"
	"math"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// plannerTracer is the package-level tracer for the reactive planner.
var plannerTracer = otel.Tracer(planning.TracerName)

// Planner is the concrete reactive planner. Stateless across calls;
// safe to share across goroutines.
type Planner struct{}

// NewPlanner returns a reactive planner with default settings. There
// are no knobs today — all per-call options come through
// [planning.Options].
func NewPlanner() *Planner { return &Planner{} }

// Name is the planner's extension identifier — the value an agent's
// [core.AgentConfig.PlannerName] must match to select this planner.
func (p *Planner) Name() string { return planning.ReactivePlannerName }

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
	domain *planning.Domain,
	goal *core.Goal,
	options planning.Options,
) (result *planning.Plan, err error) {
	if err = domain.ValidatePlanInputs(start, goal); err != nil {
		return nil, err
	}

	_, span := plannerTracer.Start(ctx, planning.ReactivePlannerName+".plan",
		trace.WithAttributes(
			attribute.String(planning.PlannerNameKey, p.Name()),
			attribute.String(planning.GoalNameKey, goal.Name()),
		),
	)
	defer func() {
		if result != nil {
			span.SetAttributes(attribute.Int(planning.PlanLengthKey, len(result.Actions())))
		}
		span.End()
	}()

	if goal.SatisfiedBy(start) {
		result = planning.NewPlan(nil, goal)
		return result, nil
	}
	best := p.bestApplicable(start, domain.Actions(), goal, options.ExcludedActions)
	if best == nil {
		return nil, nil
	}
	result = planning.NewPlan([]core.Action{best}, goal)
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
// tie. Use [core.FixedScore](v) to attach a constant cost; the canonical
// [core.NewAction] constructor fills in [core.FixedScore](1.0) when none
// is supplied.
func (p *Planner) bestApplicable(
	start core.WorldState,
	actions []core.Action,
	goal *core.Goal,
	excluded map[string]struct{},
) core.Action {
	state := start.Conditions()
	unsatisfied := p.unsatisfiedPreconditions(state, goal)

	var best core.Action
	bestProgress := 0
	bestCost := math.Inf(1)

	for _, action := range actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		if _, skip := excluded[metadata.Name]; skip {
			continue
		}
		if !metadata.Applicable(state) {
			continue
		}

		progress := p.progressTowardsGoal(metadata.Effects, unsatisfied)
		if progress == 0 {
			continue
		}

		cost := math.Inf(1)
		if metadata.Cost != nil {
			cost = metadata.Cost(start)
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
func (p *Planner) progressTowardsGoal(effects core.ConditionSet, unsatisfied map[string]core.Truth) int {
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
func (p *Planner) unsatisfiedPreconditions(state map[string]core.Truth, goal *core.Goal) map[string]core.Truth {
	out := make(map[string]core.Truth, len(goal.Preconditions()))
	for key, required := range goal.Preconditions() {
		if state[key] != required {
			out[key] = required
		}
	}
	return out
}
