package goap

import (
	"container/heap"
	"context"
	"math"
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
// expansions cap; matches embabel). Per-call overrides go through
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
// preconditions) get expanded first. Specificity-first matches embabel's
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

// search holds the state of one A* run over world states: the inputs
// (start, actions, goal, iteration cap) plus the mutable frontier (open /
// closed / gScores / cameFrom). One search serves one PlanToGoal call; its
// methods (run / expand / heuristic / reconstructPath / the optimize passes)
// operate on this shared state instead of threading open/cameFrom/gScores/
// start/goal/actions through parameter lists.
type search struct {
	start   core.WorldState
	actions []core.Action
	goal    *core.Goal
	maxIter int

	startKey string
	open     *openList
	gScores  map[string]float64
	cameFrom map[string]edge
	closed   map[string]struct{}

	// iterations counts node expansions; read for the span after run.
	iterations int
}

// newSearch seeds the frontier with the start state. The goalReachable
// pre-check and run both operate on the returned search.
func newSearch(start core.WorldState, actions []core.Action, goal *core.Goal, maxIter int) *search {
	startKey := start.HashKey()
	s := &search{
		start:    start,
		actions:  actions,
		goal:     goal,
		maxIter:  maxIter,
		startKey: startKey,
		open:     &openList{},
		gScores:  map[string]float64{startKey: 0},
		cameFrom: map[string]edge{},
		closed:   map[string]struct{}{},
	}
	heap.Init(s.open)
	heap.Push(s.open, &searchNode{state: start, gScore: 0, fScore: s.heuristic(start)})
	return s
}

// run executes the A* loop, returning the cheapest goal-satisfying node found
// (nil when none within the iteration cap). The cameFrom trail is retained on
// the search for reconstructPath; ctx cancellation aborts mid-search.
func (s *search) run(ctx context.Context) (*searchNode, error) {
	var bestGoalNode *searchNode
	bestGoalCost := math.Inf(1)

	for s.open.Len() > 0 && s.iterations < s.maxIter {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.iterations++

		current := heap.Pop(s.open).(*searchNode)
		key := current.state.HashKey()
		if _, seen := s.closed[key]; seen {
			continue
		}
		s.closed[key] = struct{}{}

		if s.goal.IsSatisfiedBy(current.state) {
			if current.gScore < bestGoalCost {
				bestGoalNode = current
				bestGoalCost = current.gScore
			}
			// Don't break — there may be cheaper paths still in the queue.
			continue
		}

		s.expand(current)
	}

	return bestGoalNode, nil
}

// expand enqueues every state reachable from current by applying one
// applicable action. The cost calc samples the start world state so
// dynamic-cost actions see a stable input across the whole search.
func (s *search) expand(current *searchNode) {
	currentKey := current.state.HashKey()
	currentState := current.state.State()
	for _, action := range s.actions {
		meta := action.Metadata()
		if !meta.IsApplicableIn(currentState) {
			continue
		}

		nextState := current.state.Apply(meta.Effects)
		nextKey := nextState.HashKey()
		if nextKey == currentKey {
			// Effect produced no observable state change in this position.
			continue
		}

		tentativeG := current.gScore
		if meta.Cost != nil {
			tentativeG += meta.Cost(s.start)
		}
		if existing, ok := s.gScores[nextKey]; ok && tentativeG >= existing {
			continue
		}

		s.gScores[nextKey] = tentativeG
		s.cameFrom[nextKey] = edge{prevKey: currentKey, action: action}

		h := s.heuristic(nextState)
		heap.Push(s.open, &searchNode{
			state:  nextState,
			gScore: tentativeG,
			fScore: tentativeG + h,
		})
	}
}

// --- A* internals ---------------------------------------------------------

type searchNode struct {
	state  core.WorldState
	gScore float64
	fScore float64
}

type openList []*searchNode

func (o openList) Len() int           { return len(o) }
func (o openList) Less(i, j int) bool { return o[i].fScore < o[j].fScore }
func (o openList) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }

func (o *openList) Push(x any) {
	*o = append(*o, x.(*searchNode))
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
	prevKey string
	action  core.Action
}

// heuristic counts unsatisfied goal preconditions. It's admissible (never
// overestimates) — every still-unsatisfied condition needs at least one
// more action to fix. That guarantees A* finds an optimal plan.
func (s *search) heuristic(worldState core.WorldState) float64 {
	state := worldState.State()
	unsatisfied := 0
	for key, required := range s.goal.Preconditions() {
		if state[key] != required {
			unsatisfied++
		}
	}
	return float64(unsatisfied)
}

// reconstructPath walks the cameFrom trail from goalKey back to the start,
// producing the action list in execution order.
func (s *search) reconstructPath(goalKey string) []core.Action {
	var reversed []core.Action
	cursor := goalKey
	for cursor != s.startKey {
		e, ok := s.cameFrom[cursor]
		if !ok {
			break
		}
		reversed = append(reversed, e.action)
		cursor = e.prevKey
	}

	slices.Reverse(reversed) // forward (execution) order
	return reversed
}

// goalReachable is a conservative one-step backward check: every still-
// unsatisfied goal precondition must be producible by at least one action
// in the candidate set. It catches the common "I forgot to register the
// action that produces X" case before A* spends 10k iterations searching
// in vain. It is intentionally NOT a full transitive reachability proof —
// such a check would need fixed-point iteration over the action graph and
// is dominated by A* itself for the common case.
func (s *search) goalReachable() bool {
	state := s.start.State()
	for key, required := range s.goal.Preconditions() {
		if state[key] == required {
			continue // already satisfied at start; no producer needed
		}
		produced := false
		for _, a := range s.actions {
			if a.Metadata().Effects[key] == required {
				produced = true
				break
			}
		}
		if !produced {
			return false
		}
	}
	return true
}

// backwardOptimize walks the plan in reverse, keeping only actions whose
// effects contribute to a still-needed condition. Tracks a "needed" set
// initialized from goal preconditions not yet satisfied at start; for each
// action we check whether it establishes any needed condition, drop it if
// not, and otherwise update needed to (needed - effects) ∪ preconditions.
//
// This catches plans where A* picked a redundant action that happens to
// have a low-cost path through it but doesn't actually produce anything
// the goal needs. forwardOptimize handles the symmetric "doesn't change
// state" case; running both passes covers redundancy from both ends.
func (s *search) backwardOptimize(actions []core.Action) []core.Action {
	if len(actions) <= 1 {
		return actions
	}

	startState := s.start.State()

	// Initialize needed = goal preconditions not yet satisfied at start.
	needed := map[string]core.Determination{}
	for key, required := range s.goal.Preconditions() {
		if startState[key] != required {
			needed[key] = required
		}
	}

	keep := make([]bool, len(actions))
	for i := len(actions) - 1; i >= 0; i-- {
		meta := actions[i].Metadata()

		// Does this action establish anything we still need?
		contributes := false
		for key, value := range meta.Effects {
			if want, ok := needed[key]; ok && want == value {
				contributes = true
				break
			}
		}
		if !contributes {
			continue
		}

		keep[i] = true

		// Effects this action establishes are no longer "needed earlier".
		for key, value := range meta.Effects {
			if want, ok := needed[key]; ok && want == value {
				delete(needed, key)
			}
		}
		// Its own preconditions become things earlier actions must establish.
		for key, required := range meta.Preconditions {
			if startState[key] == required {
				continue
			}
			needed[key] = required
		}
	}

	out := make([]core.Action, 0, len(actions))
	for i, a := range actions {
		if keep[i] {
			out = append(out, a)
		}
	}
	return out
}

// forwardOptimize replays the plan from start, dropping actions that don't
// change the world state at the point they're scheduled. This catches the
// case where A* picked an action that is logically redundant given an
// earlier action's effects (rare with the heuristic but possible).
func (s *search) forwardOptimize(actions []core.Action) []core.Action {
	if len(actions) <= 1 {
		return actions
	}

	out := make([]core.Action, 0, len(actions))
	cur := s.start
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
