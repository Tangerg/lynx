// Package goap implements the A* GOAP planner — the default planner for
// the agent runtime. The algorithm matches embabel's AStarGoapPlanner:
// search from the current world state to a state that satisfies the goal's
// preconditions, using "number of unsatisfied goal conditions" as an
// admissible heuristic (so A* is guaranteed to find an optimal plan).
package goap

import (
	"container/heap"
	"context"
	"errors"
	"math"
	"sort"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

const defaultMaxIterations = 10_000

var plannerTracer = otel.Tracer("lynx/agent/planner")

// AStarPlanner is the concrete planner. It's stateless across PlanToGoal
// calls; safe to share across goroutines.
type AStarPlanner struct {
	maxIterations int
}

// NewAStarPlanner returns a planner with sensible defaults (10k node
// expansions cap; matches embabel).
func NewAStarPlanner() *AStarPlanner {
	return &AStarPlanner{maxIterations: defaultMaxIterations}
}

// WithMaxIterations overrides the default cap. Use sparingly — large
// search spaces are usually a sign of an under-constrained agent rather
// than an algorithm tuning problem.
func (p *AStarPlanner) WithMaxIterations(n int) *AStarPlanner {
	clone := *p
	clone.maxIterations = n
	return &clone
}

// PlanToGoal is the workhorse. It does a forward A* search over world
// states.
func (p *AStarPlanner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	goal *core.Goal,
	opts plan.PlanOptions,
) (*plan.Plan, error) {
	if start == nil {
		return nil, errors.New("PlanToGoal: start WorldState is nil")
	}
	if goal == nil {
		return nil, errors.New("PlanToGoal: goal is nil")
	}
	if system == nil {
		return nil, errors.New("PlanToGoal: PlanningSystem is nil")
	}

	ctx, span := plannerTracer.Start(ctx, "lynx.agent.planner.astar",
		trace.WithAttributes(
			attribute.String("lynx.agent.goal.name", goal.Name),
			attribute.Int("lynx.agent.actions.count", len(system.Actions)),
		),
	)
	defer span.End()

	if isGoalSatisfied(start, goal) {
		span.SetAttributes(attribute.Bool("lynx.agent.astar.already_satisfied", true))
		return &plan.Plan{Actions: nil, Goal: goal}, nil
	}

	candidates := candidateActions(system.Actions, opts.ExcludedActions)
	bestGoalNode, cameFrom, iterations, err := p.searchForGoal(ctx, start, candidates, goal, p.iterationCap(opts))
	if err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.Int("lynx.agent.astar.iterations", iterations))

	if bestGoalNode == nil {
		span.SetAttributes(attribute.Bool("lynx.agent.astar.found", false))
		return nil, nil
	}

	path := reconstructPath(cameFrom, bestGoalNode.state.HashKey(), start.HashKey())
	path = forwardOptimize(path, start)

	span.SetAttributes(
		attribute.Bool("lynx.agent.astar.found", true),
		attribute.Int("lynx.agent.astar.plan_length", len(path)),
	)
	return &plan.Plan{Actions: path, Goal: goal}, nil
}

// iterationCap honors per-call MaxIterations when supplied, otherwise
// returns the planner-default.
func (p *AStarPlanner) iterationCap(opts plan.PlanOptions) int {
	if opts.MaxIterations > 0 {
		return opts.MaxIterations
	}
	return p.maxIterations
}

// candidateActions filters the master action list against the per-call
// exclusion set and stable-sorts so more-specific actions (those with more
// preconditions) get expanded first. Specificity-first matches embabel's
// behavior and keeps the search frontier focused.
func candidateActions(actions []core.Action, excluded map[string]struct{}) []core.Action {
	out := make([]core.Action, 0, len(actions))
	for _, a := range actions {
		if _, skip := excluded[a.Metadata().Name]; skip {
			continue
		}
		out = append(out, a)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].Metadata().Preconditions) > len(out[j].Metadata().Preconditions)
	})
	return out
}

// searchForGoal runs the A* loop. It's separated from PlanToGoal so the
// outer function stays focused on validation, span management, and post-
// processing.
func (p *AStarPlanner) searchForGoal(
	ctx context.Context,
	start core.WorldState,
	actions []core.Action,
	goal *core.Goal,
	maxIter int,
) (*searchNode, map[string]edge, int, error) {
	startKey := start.HashKey()
	startHeuristic := heuristic(start, goal)

	open := &openList{}
	heap.Init(open)
	heap.Push(open, &searchNode{
		state:  start,
		gScore: 0,
		hScore: startHeuristic,
		fScore: startHeuristic,
	})

	gScores := map[string]float64{startKey: 0}
	cameFrom := map[string]edge{}
	closed := map[string]struct{}{}

	var bestGoalNode *searchNode
	bestGoalCost := math.Inf(1)
	iterations := 0

	for open.Len() > 0 && iterations < maxIter {
		if err := ctx.Err(); err != nil {
			return nil, nil, iterations, err
		}
		iterations++

		current := heap.Pop(open).(*searchNode)
		key := current.state.HashKey()
		if _, seen := closed[key]; seen {
			continue
		}
		closed[key] = struct{}{}

		if isGoalSatisfied(current.state, goal) {
			if current.gScore < bestGoalCost {
				bestGoalNode = current
				bestGoalCost = current.gScore
			}
			// Don't break — there may be cheaper paths still in the queue.
			continue
		}

		expandNeighbors(current, actions, start, gScores, cameFrom, open, goal)
	}

	return bestGoalNode, cameFrom, iterations, nil
}

// expandNeighbors enqueues every state reachable from current by applying
// one applicable action. The cost calc samples the start world state so
// dynamic-cost actions see a stable input across the whole search.
func expandNeighbors(
	current *searchNode,
	actions []core.Action,
	start core.WorldState,
	gScores map[string]float64,
	cameFrom map[string]edge,
	open *openList,
	goal *core.Goal,
) {
	currentKey := current.state.HashKey()
	for _, action := range actions {
		if !isApplicable(action, current.state) {
			continue
		}

		nextState := current.state.Apply(action.Metadata().Effects)
		nextKey := nextState.HashKey()
		if nextKey == currentKey {
			// Effect produced no observable state change in this position.
			continue
		}

		tentativeG := current.gScore + action.Metadata().Cost(start)
		if existing, ok := gScores[nextKey]; ok && tentativeG >= existing {
			continue
		}

		gScores[nextKey] = tentativeG
		cameFrom[nextKey] = edge{prevKey: currentKey, prevState: current.state, action: action}

		h := heuristic(nextState, goal)
		heap.Push(open, &searchNode{
			state:  nextState,
			gScore: tentativeG,
			hScore: h,
			fScore: tentativeG + h,
		})
	}
}

// PlansToGoals generates a plan per goal and orders by NetValue desc.
func (p *AStarPlanner) PlansToGoals(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	opts plan.PlanOptions,
) ([]*plan.Plan, error) {
	if system == nil {
		return nil, errors.New("PlansToGoals: PlanningSystem is nil")
	}

	out := make([]*plan.Plan, 0, len(system.Goals))
	for _, goal := range system.Goals {
		pl, err := p.PlanToGoal(ctx, start, system, goal, opts)
		if err != nil {
			return nil, err
		}
		if pl == nil {
			continue
		}
		out = append(out, pl)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].NetValue(start) > out[j].NetValue(start)
	})
	return out, nil
}

// BestValuePlan is the tick-time entry — return the highest-value plan.
func (p *AStarPlanner) BestValuePlan(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	opts plan.PlanOptions,
) (*plan.Plan, error) {
	plans, err := p.PlansToGoals(ctx, start, system, opts)
	if err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		return nil, nil
	}
	return plans[0], nil
}

// Prune is currently a no-op — embabel's pruning is small wins on cold-
// start and not yet worth the complexity here. Returns the input unchanged
// so the runtime can call it unconditionally.
func (p *AStarPlanner) Prune(system *plan.PlanningSystem) *plan.PlanningSystem {
	return system
}

// --- A* internals ---------------------------------------------------------

type searchNode struct {
	state  core.WorldState
	gScore float64
	hScore float64
	fScore float64
	index  int
}

type openList []*searchNode

func (o openList) Len() int           { return len(o) }
func (o openList) Less(i, j int) bool { return o[i].fScore < o[j].fScore }

func (o openList) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
	o[i].index = i
	o[j].index = j
}

func (o *openList) Push(x any) {
	node := x.(*searchNode)
	node.index = len(*o)
	*o = append(*o, node)
}

func (o *openList) Pop() any {
	old := *o
	last := len(old) - 1
	node := old[last]
	old[last] = nil
	*o = old[:last]
	return node
}

type edge struct {
	prevKey   string
	prevState core.WorldState
	action    core.Action
}

// heuristic counts unsatisfied goal preconditions. It's admissible (never
// overestimates) — every still-unsatisfied condition needs at least one
// more action to fix. That guarantees A* finds an optimal plan.
func heuristic(ws core.WorldState, goal *core.Goal) float64 {
	state := ws.State()
	unsatisfied := 0
	for key, required := range goal.Preconditions() {
		if state[key] != required {
			unsatisfied++
		}
	}
	return float64(unsatisfied)
}

// isGoalSatisfied returns true when every goal precondition matches the
// current state's value at that key.
func isGoalSatisfied(ws core.WorldState, goal *core.Goal) bool {
	state := ws.State()
	for key, required := range goal.Preconditions() {
		if state[key] != required {
			return false
		}
	}
	return true
}

// isApplicable checks whether all of action's preconditions hold in ws.
// Unknown preconditions block — we don't speculatively apply.
func isApplicable(action core.Action, ws core.WorldState) bool {
	state := ws.State()
	for key, required := range action.Metadata().Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}

// reconstructPath walks the cameFrom map from goal back to start,
// producing the action list in execution order.
func reconstructPath(cameFrom map[string]edge, goalKey, startKey string) []core.Action {
	var reversed []core.Action
	cursor := goalKey
	for cursor != startKey {
		e, ok := cameFrom[cursor]
		if !ok {
			break
		}
		reversed = append(reversed, e.action)
		cursor = e.prevKey
	}

	// Reverse in place to get forward order.
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed
}

// forwardOptimize replays the plan from start, dropping actions that don't
// change the world state at the point they're scheduled. This catches the
// case where A* picked an action that is logically redundant given an
// earlier action's effects (rare with the heuristic but possible).
func forwardOptimize(actions []core.Action, start core.WorldState) []core.Action {
	if len(actions) <= 1 {
		return actions
	}

	out := make([]core.Action, 0, len(actions))
	cur := start
	for _, action := range actions {
		next := cur.Apply(action.Metadata().Effects)
		if next.HashKey() == cur.HashKey() {
			continue
		}
		out = append(out, action)
		cur = next
	}
	return out
}
