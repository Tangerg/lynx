package utility

import (
	"context"
	"math"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// OpenEndedGoalName is the conventional name for the unsatisfiable
// "keep firing useful things" goal. A planner finding a goal
// with this exact Name treats it as never-satisfied and emits a
// one-action plan as long as any applicable action remains.
//
// Combine with a real goal under [NewGoalFirst] for "iterate
// until done" pipelines: the real goal terminates; the open-ended goal keeps
// producing opportunistic work until then.
const OpenEndedGoalName = "lynx:agent:goal:open-ended"

// IsOpenEnded reports whether goal is the conventional unsatisfiable goal.
func IsOpenEnded(goal *core.Goal) bool {
	return goal != nil && goal.Name() == OpenEndedGoalName
}

const attrAnyApplicable = "agent.actions.any_applicable"

var plannerTracer = otel.Tracer(planning.TracerName)

// Planner is the classic Utility AI planner: pick the highest-
// net-value applicable action, return it as a one-step plan when
// the goal is satisfiable in one step, otherwise return nil. See
// the package doc for the difference between this and [GoalFirst].
//
// Stateless — safe to share across goroutines.
type Planner struct{}

// NewPlanner returns a Utility AI planner. There are no knobs;
// per-call configuration flows through [planning.Options].
func NewPlanner() *Planner { return &Planner{} }

// Name is the planner's extension identifier — agents select this
// planner by setting [core.AgentConfig.PlannerName] to "utility".
func (p *Planner) Name() string { return planning.UtilityPlannerName }

// PlanToGoal implements the classic shape: pick the top-net-value
// applicable action and emit a one-step plan when it reaches the
// goal (or whenever the goal is open-ended). When no applicable action
// exists, return the empty plan only if the goal is already
// satisfied.
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	domain *planning.Domain,
	goal *core.Goal,
	options planning.Options,
) (*planning.Plan, error) {
	return planUtility(ctx, p.Name(), start, domain, goal, options, false)
}

// GoalFirst is the goal-satisfaction-first variant: checks "is
// goal already satisfied?" BEFORE selecting an action, so the
// process terminates cleanly the moment the real goal lands rather
// than continuing to pick incidental high-value actions. Pairs with
// an open-ended goal for iterate-then-stop pipelines.
type GoalFirst struct{}

// NewGoalFirst returns a goal-first utility planner.
func NewGoalFirst() *GoalFirst { return &GoalFirst{} }

// Name identifies this planner: "goal-first-utility".
func (p *GoalFirst) Name() string { return planning.GoalFirstUtilityPlannerName }

// PlanToGoal differs from [Planner.PlanToGoal] only on the
// satisfied-goal short-circuit: when goal.SatisfiedBy(start) the
// planner returns the empty plan immediately, even if other
// applicable actions exist. The empty plan's net value (0) beats
// any negative-value action the open-ended goal might still produce, so the
// process picks termination over opportunistic noise.
func (p *GoalFirst) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	domain *planning.Domain,
	goal *core.Goal,
	options planning.Options,
) (*planning.Plan, error) {
	return planUtility(ctx, p.Name(), start, domain, goal, options, true)
}

// planUtility holds the shared 1-step-lookahead body for both
// utility planners. goalFirst toggles the [GoalFirst]
// short-circuit: when true and the real goal already satisfies the
// start state, return the empty plan even if higher-value actions
// remain applicable.
func planUtility(
	ctx context.Context,
	name string,
	start core.WorldState,
	domain *planning.Domain,
	goal *core.Goal,
	options planning.Options,
	goalFirst bool,
) (result *planning.Plan, err error) {
	if err = domain.ValidatePlanInputs(start, goal); err != nil {
		return nil, err
	}
	_, span := plannerTracer.Start(ctx, name+".plan",
		trace.WithAttributes(
			attribute.String(planning.PlannerNameKey, name),
			attribute.String(planning.GoalNameKey, goal.Name()),
		),
	)
	defer func() {
		if result != nil {
			span.SetAttributes(attribute.Int(planning.PlanLengthKey, len(result.Actions())))
		}
		span.End()
	}()

	firstAction := topApplicable(start, domain.Actions(), options.ExcludedActions)
	span.SetAttributes(attribute.Bool(attrAnyApplicable, firstAction != nil))

	if IsOpenEnded(goal) {
		if firstAction == nil {
			return nil, nil
		}
		return planning.NewPlan([]core.Action{firstAction}, goal), nil
	}

	if goalFirst && goal.SatisfiedBy(start) {
		return planning.NewPlan(nil, goal), nil
	}

	if firstAction == nil {
		if !goalFirst && goal.SatisfiedBy(start) {
			return planning.NewPlan(nil, goal), nil
		}
		return nil, nil
	}
	if goal.SatisfiedBy(start.Apply(firstAction.Metadata().Effects)) {
		return planning.NewPlan([]core.Action{firstAction}, goal), nil
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
	excluded planning.Exclusions,
) core.Action {
	state := start.Conditions()
	var (
		best      core.Action
		bestValue = math.Inf(-1)
	)
	for _, action := range actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		if excluded.Contains(metadata.Name) {
			continue
		}
		if !metadata.Applicable(state) {
			continue
		}
		value := netValue(start, metadata)
		if value > bestValue {
			best, bestValue = action, value
		}
	}
	return best
}

// netValue computes Value(state) − Cost(state). nil Cost defaults to
// 0 (no cost penalty), matching the convention that a missing cost
// is "free"; nil Value defaults to 0. NaN / Inf are squashed to 0 so
// a misbehaving value func can't poison the ordering.
func netValue(state core.WorldState, metadata core.ActionMetadata) float64 {
	value := safeScalar(metadata.Value, state)
	cost := safeScalar(metadata.Cost, state)
	return value - cost
}

func safeScalar(score func(core.WorldState) float64, state core.WorldState) float64 {
	if score == nil {
		return 0
	}
	value := score(state)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

// Compile-time assertions that the planners satisfy [planning.Planner];
// keeps the contract honest if the interface grows.
var (
	_ planning.Planner = (*Planner)(nil)
	_ planning.Planner = (*GoalFirst)(nil)
)
