package goap

import (
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

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

	slices.Reverse(reversed)
	return reversed
}

// backwardOptimize walks the plan in reverse, keeping only actions whose
// effects contribute to a still-needed condition. It catches plans where A*
// found a low-cost path through an action that doesn't produce anything the
// goal or later kept actions need.
func (s *search) backwardOptimize(actions []core.Action) []core.Action {
	if len(actions) <= 1 {
		return actions
	}

	startState := s.start.State()
	needed := map[string]core.Determination{}
	for key, required := range s.goal.Preconditions() {
		if startState[key] != required {
			needed[key] = required
		}
	}

	keep := make([]bool, len(actions))
	for i := len(actions) - 1; i >= 0; i-- {
		meta := actions[i].Metadata()

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

		for key, value := range meta.Effects {
			if want, ok := needed[key]; ok && want == value {
				delete(needed, key)
			}
		}
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
// change the world state at the point they're scheduled. Running it after
// backwardOptimize covers redundancy from both ends of the plan.
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
