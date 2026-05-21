package utility

import (
	"context"
	"math"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

// NirvanaGoalName is the conventional name for the unsatisfiable
// "keep firing useful things" goal. Mirrors embabel's
// `com.embabel.agent.core.support.NIRVANA`. A planner finding a goal
// with this exact Name treats it as never-satisfied and emits a
// one-action plan as long as any applicable action remains.
//
// Combine with a real goal under [NewHybridPlanner] for "iterate
// until done" pipelines: the real goal terminates, Nirvana keeps
// producing opportunistic work until then.
const NirvanaGoalName = "lynx:agent:goal:nirvana"

// IsNirvana reports whether g is the conventional unsatisfiable
// goal (g != nil and g.Name == NirvanaGoalName).
func IsNirvana(g *core.Goal) bool {
	return g != nil && g.Name == NirvanaGoalName
}

var plannerTracer = otel.Tracer("lynx/agent/planner")

// Planner is the classic Utility AI planner: pick the highest-
// net-value applicable action, return it as a one-step plan when
// the goal is satisfiable in one step, otherwise return nil. See
// the package doc for the difference between this and [HybridPlanner].
//
// Stateless — safe to share across goroutines.
type Planner struct{}

// NewPlanner returns a Utility AI planner. There are no knobs;
// per-call configuration flows through [plan.PlanOptions].
func NewPlanner() *Planner { return &Planner{} }

// Name is the planner's extension identifier — agents select this
// planner by setting [core.AgentConfig.PlannerName] to "utility".
func (p *Planner) Name() string { return "utility" }

// PlanToGoal implements the classic shape: pick the top-net-value
// applicable action and emit a one-step plan when it reaches the
// goal (or any time the goal is Nirvana). When no applicable action
// exists, return the empty plan only if the goal is already
// satisfied.
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	goal *core.Goal,
	options plan.PlanOptions,
) (*plan.Plan, error) {
	return planUtility(ctx, p.Name(), start, system, goal, options, false)
}

// HybridPlanner is the goal-satisfaction-first variant: checks "is
// goal already satisfied?" BEFORE selecting an action, so the
// process terminates cleanly the moment the real goal lands rather
// than continuing to pick incidental high-value actions. Pairs with
// the Nirvana goal for iterate-then-stop pipelines.
type HybridPlanner struct{}

// NewHybridPlanner returns a hybrid utility planner.
func NewHybridPlanner() *HybridPlanner { return &HybridPlanner{} }

// Name identifies this planner: "hybrid-utility".
func (p *HybridPlanner) Name() string { return "hybrid-utility" }

// PlanToGoal differs from [Planner.PlanToGoal] only on the
// satisfied-goal short-circuit: when goal.IsSatisfiedBy(start) the
// hybrid planner returns the empty plan immediately, even if other
// applicable actions exist. The empty plan's net value (0) beats
// any negative-value tail Nirvana might still produce, so the
// process picks termination over opportunistic noise.
func (p *HybridPlanner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	goal *core.Goal,
	options plan.PlanOptions,
) (*plan.Plan, error) {
	return planUtility(ctx, p.Name(), start, system, goal, options, true)
}

// planUtility holds the shared 1-step-lookahead body for both
// utility planners. satisfiedFirst toggles the [HybridPlanner]
// short-circuit: when true and the real goal already satisfies the
// start state, return the empty plan even if higher-value actions
// remain applicable.
func planUtility(
	ctx context.Context,
	name string,
	start core.WorldState,
	system *plan.PlanningSystem,
	goal *core.Goal,
	options plan.PlanOptions,
	satisfiedFirst bool,
) (result *plan.Plan, err error) {
	if err = plan.CheckPlanInputs(start, system, goal); err != nil {
		return nil, err
	}
	_, span := plannerTracer.Start(ctx, name+".plan",
		trace.WithAttributes(
			attribute.String("lynx.agent.planner", name),
			attribute.String("lynx.agent.goal.name", goal.Name),
		),
	)
	defer func() {
		if result != nil {
			span.SetAttributes(attribute.Int("lynx.agent.plan.length", len(result.Actions)))
		}
		span.End()
	}()

	first := topApplicable(start, system.Actions, options.ExcludedActions)
	span.SetAttributes(attribute.Bool("lynx.agent.actions.any_applicable", first != nil))

	if IsNirvana(goal) {
		if first == nil {
			return nil, nil
		}
		return &plan.Plan{Actions: []core.Action{first}, Goal: goal}, nil
	}

	if satisfiedFirst && goal.IsSatisfiedBy(start) {
		return &plan.Plan{Goal: goal}, nil
	}

	if first == nil {
		if !satisfiedFirst && goal.IsSatisfiedBy(start) {
			return &plan.Plan{Goal: goal}, nil
		}
		return nil, nil
	}
	if goal.IsSatisfiedBy(start.Apply(first.Metadata().Effects)) {
		return &plan.Plan{Actions: []core.Action{first}, Goal: goal}, nil
	}
	return nil, nil
}

// topApplicable returns the highest-net-value action applicable in
// start that's not in the excluded set, or nil when no candidate
// qualifies. Single-pass O(n) — both planners only need the top
// pick, so a full sort would be wasted work.
func topApplicable(
	start core.WorldState,
	actions []core.Action,
	excluded map[string]struct{},
) core.Action {
	state := start.State()
	var (
		best    core.Action
		bestVal = math.Inf(-1)
	)
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
		v := netValue(start, meta)
		if v > bestVal {
			best, bestVal = action, v
		}
	}
	return best
}

// netValue computes Value(state) − Cost(state). nil Cost defaults to
// 0 (no cost penalty), matching the convention that a missing cost
// is "free"; nil Value defaults to 0. NaN / Inf are squashed to 0 so
// a misbehaving value func can't poison the ordering.
func netValue(start core.WorldState, meta core.ActionMetadata) float64 {
	value := safeScalar(meta.Value, start)
	cost := safeScalar(meta.Cost, start)
	return value - cost
}

func safeScalar(f func(core.WorldState) float64, s core.WorldState) float64 {
	if f == nil {
		return 0
	}
	v := f(s)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// Compile-time assertions that the planners satisfy [plan.Planner];
// keeps the contract honest if the interface grows.
var (
	_ plan.Planner = (*Planner)(nil)
	_ plan.Planner = (*HybridPlanner)(nil)
)
