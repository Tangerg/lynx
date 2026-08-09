package goap

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/lynx/agent/planning"
)

type searchNode struct {
	state planning.WorldState
	cost  float64
	order uint64
}

type frontier []*searchNode

func (values frontier) Len() int { return len(values) }

func (values frontier) Less(left, right int) bool {
	if values[left].cost != values[right].cost {
		return values[left].cost < values[right].cost
	}
	return values[left].order < values[right].order
}

func (values frontier) Swap(left, right int) {
	values[left], values[right] = values[right], values[left]
}

func (values *frontier) Push(value any) { *values = append(*values, value.(*searchNode)) }

func (values *frontier) Pop() any {
	old := *values
	last := len(old) - 1
	value := old[last]
	old[last] = nil
	*values = old[:last]
	return value
}

type predecessor struct {
	stateKey string
	action   planning.PlannedAction
}

type search struct {
	problem       planning.Problem
	maxExpansions uint32
	startKey      string
	frontier      *frontier
	bestCosts     map[string]float64
	predecessors  map[string]predecessor
	nextOrder     uint64
	expansions    uint32
}

func newSearch(problem planning.Problem, maxExpansions uint32) *search {
	start := problem.InitialState()
	queue := &frontier{}
	heap.Init(queue)
	search := &search{
		problem: problem, maxExpansions: maxExpansions, startKey: start.Key(), frontier: queue,
		bestCosts: map[string]float64{start.Key(): 0}, predecessors: make(map[string]predecessor),
	}
	search.push(start, 0)
	return search
}

func (search *search) push(state planning.WorldState, cost float64) {
	heap.Push(search.frontier, &searchNode{state: state, cost: cost, order: search.nextOrder})
	search.nextOrder++
}

func (search *search) run(ctx context.Context) (searchNode, bool, error) {
	for search.frontier.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return searchNode{}, false, err
		}
		current := heap.Pop(search.frontier).(*searchNode)
		currentKey := current.state.Key()
		if current.cost != search.bestCosts[currentKey] {
			continue
		}
		if search.expansions == search.maxExpansions {
			return searchNode{}, false, fmt.Errorf("%w: %d", ErrExpansionLimitReached, search.maxExpansions)
		}
		search.expansions++
		if search.problem.Goal().SatisfiedBy(current.state) {
			return *current, true, nil
		}
		if err := search.expand(current); err != nil {
			return searchNode{}, false, err
		}
	}
	return searchNode{}, false, nil
}

func (search *search) expand(current *searchNode) error {
	currentKey := current.state.Key()
	for _, action := range search.problem.Actions() {
		if !action.Applicable(current.state) {
			continue
		}
		nextState, err := action.Apply(current.state)
		if err != nil {
			return fmt.Errorf("goap: apply Action %q at state %q: %w", action.Name(), currentKey, err)
		}
		nextKey := nextState.Key()
		if nextKey == currentKey {
			continue
		}
		edgeCost, err := action.Cost(current.state)
		if err != nil {
			return fmt.Errorf("goap: Action %q at state %q: %w", action.Name(), currentKey, err)
		}
		cost := current.cost + edgeCost
		if math.IsInf(cost, 0) {
			return fmt.Errorf("%w: Action %q overflows cumulative cost", planning.ErrInvalidActionCost, action.Name())
		}
		if best, known := search.bestCosts[nextKey]; known && cost >= best {
			continue
		}
		planned, err := planning.NewPlannedAction(action.Name())
		if err != nil {
			return err
		}
		search.bestCosts[nextKey] = cost
		search.predecessors[nextKey] = predecessor{stateKey: currentKey, action: planned}
		search.push(nextState, cost)
	}
	return nil
}

func (search *search) reconstruct(goalKey string) ([]planning.PlannedAction, error) {
	var reversed []planning.PlannedAction
	for cursor := goalKey; cursor != search.startKey; {
		previous, found := search.predecessors[cursor]
		if !found {
			return nil, fmt.Errorf("goap: predecessor missing for state %q", cursor)
		}
		reversed = append(reversed, previous.action)
		cursor = previous.stateKey
	}
	slices.Reverse(reversed)
	return reversed, nil
}

func (search *search) hasGoalProducers() bool {
	initial := search.problem.InitialState()
	for _, required := range search.problem.Goal().Conditions() {
		if initial.Truth(required.Key()) == required.Truth() {
			continue
		}
		produced := false
		for _, action := range search.problem.Actions() {
			for _, effect := range action.Effects() {
				if effect.Key() == required.Key() && effect.Truth() == required.Truth() {
					produced = true
					break
				}
			}
			if produced {
				break
			}
		}
		if !produced {
			return false
		}
	}
	return true
}
