package reactive

import (
	"context"
	"fmt"
	"math"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/score"
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
// unsatisfied goal requirements its effects would close, picks the
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

	unsatisfied, err := domain.Unsatisfied(ctx, start, goal.Requirements(), options.ConditionResolver)
	if err != nil {
		return nil, err
	}
	if len(unsatisfied) == 0 {
		result = planning.NewPlan(nil, goal)
		return result, nil
	}
	best, err := p.bestApplicable(ctx, start, domain, unsatisfied, options)
	if err != nil {
		return nil, err
	}
	if best == nil {
		return nil, nil
	}
	result = planning.NewPlan([]core.Action{best}, goal)
	return result, nil
}

// bestApplicable picks the action whose effects close the most
// still-unsatisfied goal requirements; ties broken by lower cost.
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
	ctx context.Context,
	start core.WorldState,
	domain *planning.Domain,
	unsatisfied core.ConditionSet,
	options planning.Options,
) (core.Action, error) {
	candidates := make([]core.Action, 0, len(domain.Actions()))
	for _, action := range domain.Actions() {
		if nilvalue.Is(action) {
			continue
		}
		metadata := action.Metadata()
		if options.ExcludedActions.Contains(metadata.Name) || p.progressTowardsGoal(metadata.Effects, unsatisfied) == 0 {
			continue
		}
		candidates = append(candidates, action)
	}
	applicable, err := domain.ApplicableActions(ctx, start, candidates, options.ConditionResolver)
	if err != nil {
		return nil, err
	}
	scoreState, err := domain.StateWithResolvedConditions(start, options.ConditionResolver)
	if err != nil {
		return nil, err
	}

	var best core.Action
	bestProgress := 0
	bestCost := math.Inf(1)

	for _, action := range applicable {
		metadata := action.Metadata()

		progress := p.progressTowardsGoal(metadata.Effects, unsatisfied)

		cost := math.Inf(1)
		if metadata.Cost != nil {
			var err error
			cost, err = score.Evaluate(metadata.Cost, scoreState)
			if err != nil {
				return nil, fmt.Errorf("reactive: action %q cost: %w", metadata.Name, err)
			}
			if !score.Finite(cost) || cost < 0 {
				return nil, fmt.Errorf("reactive: action %q cost returned %v; cost must be finite and non-negative", metadata.Name, cost)
			}
		}

		if progress > bestProgress || (progress == bestProgress && cost < bestCost) {
			best = action
			bestProgress = progress
			bestCost = cost
		}
	}
	return best, nil
}

// progressTowardsGoal counts how many still-unsatisfied goal
// preconditions this effect map would establish.
func (p *Planner) progressTowardsGoal(effects, unsatisfied core.ConditionSet) int {
	progress := 0
	for key, required := range unsatisfied {
		if effects[key] == required {
			progress++
		}
	}
	return progress
}
