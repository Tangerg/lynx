package goap

import (
	"container/heap"
	"context"
	"math"

	"github.com/Tangerg/lynx/agent/core"
)

// search holds the state of one A* run over world states: the inputs
// (start, actions, goal, iteration cap) plus the mutable frontier (open /
// closed / gScores / cameFrom). One search serves one PlanToGoal call.
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
// within the iteration cap. It keeps searching after the first goal hit because
// a cheaper goal node may still be queued.
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
			continue
		}

		s.expand(current)
	}

	return bestGoalNode, nil
}

// expand enqueues every state reachable from current by applying one
// applicable action. Dynamic-cost actions are evaluated against the start
// state so their cost input stays stable across the whole search.
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

// heuristic counts unsatisfied goal preconditions. It is admissible: every
// still-unsatisfied condition needs at least one more action to fix.
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

// goalReachable is a conservative one-step backward check. It catches the
// common "no action can produce this goal condition" case before A* burns the
// iteration cap; it is not a full transitive reachability proof.
func (s *search) goalReachable() bool {
	state := s.start.State()
	for key, required := range s.goal.Preconditions() {
		if state[key] == required {
			continue
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
