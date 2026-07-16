package goap

import (
	"github.com/Tangerg/lynx/agent/core"
)

// relevantActions narrows actions to the transitive backward closure
// reachable from goal: an action is "relevant" iff some effect of it
// matches a (key, value) the planner still needs, where the need set
// starts at goal.Preconditions and grows to include the preconditions
// of every newly-relevant action.
//
// In planner terms: STRIPS regression / backward chaining. Any action
// outside the closure provably cannot contribute to satisfying the
// goal — its effects don't appear anywhere in the goal's transitive
// requirement graph. Pruning them shrinks A*'s expansion frontier
// without losing optimality.
//
// Returned slice preserves the input order for stable downstream
// behavior (the caller's specificity sort still applies). When goal
// is unreachable from any subset of actions, the returned slice is
// empty — the caller can short-circuit before running A*.
func relevantActions(actions []core.Action, goal *core.Goal) []core.Action {
	// needed is the set of (key, value) pairs that must hold at
	// some point in a successful plan — represented as a set of
	// tuples because the same key can be needed at different values
	// at different plan positions (e.g., action B requires X=True
	// transiently while goal wants X=False at end).
	needed := map[needTuple]struct{}{}
	for key, value := range goal.Preconditions() {
		needed[needTuple{key: key, value: value}] = struct{}{}
	}

	relevant := map[string]struct{}{}

	// Fixed point: keep adding actions to relevant until a full pass
	// produces no new entries. Adding to needed within a pass is fine —
	// the next pass picks up actions that now match the expanded need
	// set.
	for {
		changed := false
		for _, action := range actions {
			if action == nil {
				continue
			}
			metadata := action.Metadata()
			if _, found := relevant[metadata.Name]; found {
				continue
			}

			contributes := false
			for key, value := range metadata.Effects {
				if _, want := needed[needTuple{key: key, value: value}]; want {
					contributes = true
					break
				}
			}
			if !contributes {
				continue
			}

			relevant[metadata.Name] = struct{}{}
			changed = true

			for key, value := range metadata.Preconditions {
				needed[needTuple{key: key, value: value}] = struct{}{}
			}
		}
		if !changed {
			break
		}
	}

	out := make([]core.Action, 0, len(relevant))
	for _, action := range actions {
		if action == nil {
			continue
		}
		if _, in := relevant[action.Metadata().Name]; in {
			out = append(out, action)
		}
	}
	return out
}

// needTuple is a hashable (key, value) pair used as the element type
// of the regression's need-set. Plain map[string]Truth won't
// do because the same key may be needed at different values in
// different parts of a plan.
type needTuple struct {
	key   string
	value core.Truth
}
