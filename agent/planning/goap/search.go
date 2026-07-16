package goap

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

// search holds one uniform-cost run over immutable world states.
type search struct {
	start         core.WorldState
	actions       []core.Action
	goal          *core.Goal
	maxExpansions int

	startKey     string
	frontier     *frontier
	bestCosts    map[string]float64
	predecessors map[string]edge
	settled      map[string]struct{}
	nextOrder    uint64

	// expansions counts settled nodes; read for tracing after run.
	expansions int
}

// newSearch seeds a uniform-cost frontier for this planning pass.
func newSearch(
	start core.WorldState,
	actions []core.Action,
	goal *core.Goal,
	maxExpansions int,
) *search {
	startKey := start.Key()
	s := &search{
		start:         start,
		actions:       actions,
		goal:          goal,
		maxExpansions: maxExpansions,
		startKey:      startKey,
		frontier:      &frontier{},
		bestCosts:     map[string]float64{startKey: 0},
		predecessors:  map[string]edge{},
		settled:       map[string]struct{}{},
	}
	heap.Init(s.frontier)
	s.push(start, 0)
	return s
}

func (s *search) push(state core.WorldState, cost float64) {
	heap.Push(s.frontier, &searchNode{state: state, cost: cost, order: s.nextOrder})
	s.nextOrder++
}

// run returns the first goal state removed from the cost-ordered frontier.
// With non-negative edges, that node is globally cheapest.
func (s *search) run(ctx context.Context) (*searchNode, error) {
	for s.frontier.Len() > 0 && s.expansions < s.maxExpansions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		current := heap.Pop(s.frontier).(*searchNode)
		key := current.state.Key()
		if best, currentBest := s.bestCosts[key]; !currentBest || current.cost != best {
			continue // stale queue entry superseded by a cheaper path
		}
		if _, seen := s.settled[key]; seen {
			continue
		}
		s.settled[key] = struct{}{}
		s.expansions++

		if s.goal.SatisfiedBy(current.state) {
			return current, nil
		}

		if err := s.expand(current); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// expand enqueues every state reachable from current by applying one
// applicable action. A dynamic cost is evaluated against the edge's source
// state, preserving the public ScoreFunc contract.
func (s *search) expand(current *searchNode) error {
	currentKey := current.state.Key()
	currentState := current.state.Conditions()
	for _, action := range s.actions {
		metadata := action.Metadata()
		if !metadata.Applicable(currentState) {
			continue
		}

		nextState := current.state.Apply(metadata.Effects)
		nextKey := nextState.Key()
		if nextKey == currentKey {
			continue
		}

		edgeCost := 0.0
		if metadata.Cost != nil {
			edgeCost = metadata.Cost(current.state)
		}
		if math.IsNaN(edgeCost) || math.IsInf(edgeCost, 0) || edgeCost < 0 {
			return fmt.Errorf(
				"%w: action %q at state %q returned %v",
				ErrInvalidActionCost,
				metadata.Name,
				currentKey,
				edgeCost,
			)
		}
		cost := current.cost + edgeCost
		if existing, ok := s.bestCosts[nextKey]; ok && cost >= existing {
			continue
		}

		s.bestCosts[nextKey] = cost
		s.predecessors[nextKey] = edge{prevKey: currentKey, action: action}
		s.push(nextState, cost)
	}
	return nil
}

func (s *search) reconstructPath(goalKey string) ([]core.Action, error) {
	var reversed []core.Action
	for cursor := goalKey; cursor != s.startKey; {
		edge, ok := s.predecessors[cursor]
		if !ok {
			return nil, fmt.Errorf("goap: predecessor missing for state %q", cursor)
		}
		reversed = append(reversed, edge.action)
		cursor = edge.prevKey
	}
	slices.Reverse(reversed)
	return reversed, nil
}

// hasGoalProducers is a conservative direct-producer check. It catches the
// common "no action can establish this goal condition" case before search
// burns the expansion cap; it is not a transitive reachability proof.
func (s *search) hasGoalProducers() bool {
	state := s.start.Conditions()
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
