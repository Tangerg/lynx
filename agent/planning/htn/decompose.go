package htn

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// decompose recursively expands task into a flat action list,
// threading the world state through so each subtask sees the effects
// of its predecessors. Returns:
//
//   - (actions, finalState, true, nil) on success;
//   - (nil, state, false, nil) when no method applies — the caller (a
//     parent method) treats this as a backtrack signal and tries its
//     next method;
//   - (nil, _, err) on a structural error that aborts the entire
//     plan (recursion depth exceeded; method references an unknown
//     subtask; ctx canceled).
func (p *Planner) decompose(
	ctx context.Context,
	task *Task,
	state core.WorldState,
	excluded map[string]struct{},
	depth int,
) ([]core.Action, core.WorldState, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, state, false, err
	}
	if depth > p.maxRecursion {
		return nil, state, false, fmt.Errorf("htn.Planner.decompose: exceeded max recursion depth %d at task %q", p.maxRecursion, task.Name)
	}

	if task.IsPrimitive() {
		metadata := task.Action.Metadata()
		if _, skip := excluded[metadata.Name]; skip {
			return nil, state, false, nil
		}
		return []core.Action{task.Action}, state.Apply(metadata.Effects), true, nil
	}

	// Snapshot once so all method-applicability probes share the same
	// view without paying one defensive map copy per method.
	stateMap := state.Conditions()

	for _, method := range task.Methods {
		if !methodApplicable(method, stateMap) {
			continue
		}
		actions, next, ok, err := p.tryMethod(ctx, method, state, excluded, depth)
		if err != nil {
			return nil, state, false, err
		}
		if ok {
			return actions, next, true, nil
		}
	}
	return nil, state, false, nil
}

// tryMethod walks a method's subtask list, accumulating actions and
// threading state. Failure of any subtask aborts the method; the
// caller then tries the next method.
func (p *Planner) tryMethod(
	ctx context.Context,
	method Method,
	state core.WorldState,
	excluded map[string]struct{},
	depth int,
) ([]core.Action, core.WorldState, bool, error) {
	actions := make([]core.Action, 0, len(method.Subtasks))
	cur := state
	for _, subtaskName := range method.Subtasks {
		sub, ok := p.library.Lookup(subtaskName)
		if !ok {
			return nil, state, false, fmt.Errorf("htn.Planner.tryMethod: method %q references unknown subtask %q", method.Name, subtaskName)
		}
		subActions, next, ok, err := p.decompose(ctx, sub, cur, excluded, depth+1)
		if err != nil {
			return nil, state, false, err
		}
		if !ok {
			return nil, state, false, nil
		}
		actions = append(actions, subActions...)
		cur = next
	}
	return actions, cur, true, nil
}

// methodApplicable reports whether every method precondition holds in
// the supplied state map.
func methodApplicable(method Method, state map[string]core.Truth) bool {
	for key, required := range method.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}
