package utility

import (
	"context"
	"math"
	"sort"

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

// PlanToGoal implements the classic shape:
//
//   - Nirvana goal: emit a one-step plan with the highest-net-value
//     applicable action, or nil when no action is applicable.
//   - Real goal, no applicable actions: empty plan if already
//     satisfied, else nil.
//   - Real goal with applicable actions: 1-step plan when the
//     top-pick reaches the goal, else nil.
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
	_, span := plannerTracer.Start(ctx, "utility.plan",
		trace.WithAttributes(
			attribute.String("lynx.agent.planner", "utility"),
			attribute.String("lynx.agent.goal.name", goal.Name),
		),
	)
	defer func() {
		if result != nil {
			span.SetAttributes(attribute.Int("lynx.agent.plan.length", len(result.Actions)))
		}
		span.End()
	}()

	ranked := rankApplicable(start, system.Actions, options.ExcludedActions)
	span.SetAttributes(attribute.Int("lynx.agent.actions.applicable", len(ranked)))

	first := topAction(ranked)

	if IsNirvana(goal) {
		if first == nil {
			return nil, nil
		}
		return &plan.Plan{Actions: []core.Action{first}, Goal: goal}, nil
	}

	if first == nil {
		// No applicable actions — only "already satisfied" can win.
		if goal.IsSatisfiedBy(start) {
			return &plan.Plan{Goal: goal}, nil
		}
		return nil, nil
	}

	// Single-step lookahead.
	after := apply(start, first)
	if goal.IsSatisfiedBy(after) {
		return &plan.Plan{Actions: []core.Action{first}, Goal: goal}, nil
	}
	return nil, nil
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
) (result *plan.Plan, err error) {
	if err = plan.CheckPlanInputs(start, system, goal); err != nil {
		return nil, err
	}
	_, span := plannerTracer.Start(ctx, "hybrid-utility.plan",
		trace.WithAttributes(
			attribute.String("lynx.agent.planner", "hybrid-utility"),
			attribute.String("lynx.agent.goal.name", goal.Name),
		),
	)
	defer func() {
		if result != nil {
			span.SetAttributes(attribute.Int("lynx.agent.plan.length", len(result.Actions)))
		}
		span.End()
	}()

	ranked := rankApplicable(start, system.Actions, options.ExcludedActions)
	first := topAction(ranked)

	if IsNirvana(goal) {
		if first == nil {
			return nil, nil
		}
		return &plan.Plan{Actions: []core.Action{first}, Goal: goal}, nil
	}

	// Hybrid contract: check satisfaction FIRST.
	if goal.IsSatisfiedBy(start) {
		return &plan.Plan{Goal: goal}, nil
	}

	if first == nil {
		return nil, nil
	}
	after := apply(start, first)
	if goal.IsSatisfiedBy(after) {
		return &plan.Plan{Actions: []core.Action{first}, Goal: goal}, nil
	}
	return nil, nil
}

// --- shared helpers ---------------------------------------------------------

// scoredAction pairs an action with its net value at the planning
// instant. Used by [rankApplicable] for descending-net-value sort.
type scoredAction struct {
	action   core.Action
	netValue float64
}

// rankApplicable filters actions to those applicable in start and
// not excluded, then sorts by net value descending. Returns a fresh
// slice; the input is not mutated.
func rankApplicable(
	start core.WorldState,
	actions []core.Action,
	excluded map[string]struct{},
) []scoredAction {
	state := start.State()
	out := make([]scoredAction, 0, len(actions))
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
		out = append(out, scoredAction{
			action:   action,
			netValue: netValue(start, meta),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].netValue > out[j].netValue
	})
	return out
}

// topAction returns the highest-net-value action, or nil when the
// ranked slice is empty.
func topAction(ranked []scoredAction) core.Action {
	if len(ranked) == 0 {
		return nil
	}
	return ranked[0].action
}

// netValue computes Value(state) − Cost(state). nil Cost defaults to
// 0 (no cost penalty), matching the convention that a missing cost
// is "free"; nil Value defaults to 0. The planner ranks actions by
// this scalar — higher is better. Use [core.Static] for constant
// numbers.
func netValue(start core.WorldState, meta core.ActionMetadata) float64 {
	var value, cost float64
	if meta.Value != nil {
		value = meta.Value(start)
	}
	if meta.Cost != nil {
		cost = meta.Cost(start)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}
	if math.IsNaN(cost) || math.IsInf(cost, 0) {
		cost = 0
	}
	return value - cost
}

// apply returns the world state after action's effects have been
// merged on top of start. Used by the 1-step-lookahead branch of
// both planners.
func apply(start core.WorldState, action core.Action) core.WorldState {
	return start.Apply(action.Metadata().Effects)
}
