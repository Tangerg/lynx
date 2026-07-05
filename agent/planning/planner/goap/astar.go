package goap

import (
	"context"
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

const defaultMaxIterations = 10_000

// Tracing span / attribute keys for the A* planner. Centralized so a
// typo at one call site is impossible and listeners have one schema to
// key off; treat as stable across releases.
const (
	spanAstar = "agent.planner.astar"

	attrGoalName           = "agent.goal.name"
	attrActionsCount       = "agent.actions.count"
	attrAstarAlreadySat    = "agent.astar.already_satisfied"
	attrAstarReachable     = "agent.astar.reachable"
	attrAstarIterations    = "agent.astar.iterations"
	attrAstarFound         = "agent.astar.found"
	attrAstarPlanLength    = "agent.astar.plan_length"
	attrAstarPlanLengthRaw = "agent.astar.plan_length_raw"
)

var plannerTracer = otel.Tracer("lynx/agent/planner")

// Planner is the concrete planner. It's stateless across PlanToGoal
// calls; safe to share across goroutines.
type Planner struct {
	maxIterations int
}

// NewPlanner returns a planner with sensible defaults (10k node
// expansions cap). Per-call overrides go through
// [planning.Options].MaxIterations.
func NewPlanner() *Planner {
	return &Planner{maxIterations: defaultMaxIterations}
}

// Name is the planner's extension identifier — the value an agent's
// [core.AgentConfig.PlannerName] must match to select this planner.
func (p *Planner) Name() string { return "goap" }

// PlanToGoal is the workhorse. It does a forward A* search over world
// states.
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	system *planning.System,
	goal *core.Goal,
	options planning.Options,
) (*planning.Plan, error) {
	if err := planning.CheckPlanInputs(start, system, goal); err != nil {
		return nil, err
	}

	ctx, span := plannerTracer.Start(ctx, spanAstar,
		trace.WithAttributes(
			attribute.String(attrGoalName, goal.Name),
			attribute.Int(attrActionsCount, len(system.Actions)),
		),
	)
	defer span.End()

	if goal.IsSatisfiedBy(start) {
		span.SetAttributes(attribute.Bool(attrAstarAlreadySat, true))
		return &planning.Plan{Actions: nil, Goal: goal}, nil
	}

	candidates := candidateActions(system.Actions, options.ExcludedActions)

	// Backward relevance pruning: keep only actions in the goal's
	// transitive requirement graph. STRIPS regression — provably safe
	// (an excluded action's effects don't appear in any condition
	// reachable backward from the goal) and shrinks A*'s expansion
	// frontier substantially on agents with many domain-specific
	// actions whose effects don't interact with the current goal.
	candidates = relevantActions(candidates, goal)

	s := newSearch(start, candidates, goal, p.iterationCap(options))

	// Reachability pre-check — short-circuits before A* burns 10k iterations
	// chasing a goal whose required conditions no action can establish.
	// After pruning the check operates on the regression set, so a goal
	// precondition with no producer in the relevant closure is caught here
	// even when the unpruned action set had a "producer" whose own
	// preconditions can never be met.
	if !s.goalReachable() {
		span.SetAttributes(attribute.Bool(attrAstarReachable, false))
		return nil, nil
	}

	bestGoalNode, err := s.run(ctx)
	if err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.Int(attrAstarIterations, s.iterations))

	if bestGoalNode == nil {
		span.SetAttributes(attribute.Bool(attrAstarFound, false))
		return nil, nil
	}

	path := s.reconstructPath(bestGoalNode.state.HashKey())
	rawLen := len(path)
	path = s.backwardOptimize(path)
	path = s.forwardOptimize(path)

	span.SetAttributes(
		attribute.Bool(attrAstarFound, true),
		attribute.Int(attrAstarPlanLengthRaw, rawLen),
		attribute.Int(attrAstarPlanLength, len(path)),
	)
	return &planning.Plan{Actions: path, Goal: goal}, nil
}

// iterationCap honors per-call MaxIterations when supplied, otherwise
// returns the planner-default.
func (p *Planner) iterationCap(options planning.Options) int {
	if options.MaxIterations > 0 {
		return options.MaxIterations
	}
	return p.maxIterations
}

// candidateActions filters the master action list against the per-call
// exclusion set and stable-sorts so more-specific actions (those with more
// preconditions) get expanded first. Specificity-first
// behavior and keeps the search frontier focused.
func candidateActions(actions []core.Action, excluded map[string]struct{}) []core.Action {
	out := make([]core.Action, 0, len(actions))
	for _, action := range actions {
		if action == nil {
			continue
		}
		if _, skip := excluded[action.Metadata().Name]; skip {
			continue
		}
		out = append(out, action)
	}

	slices.SortStableFunc(out, func(a, b core.Action) int {
		return len(b.Metadata().Preconditions) - len(a.Metadata().Preconditions)
	})
	return out
}
