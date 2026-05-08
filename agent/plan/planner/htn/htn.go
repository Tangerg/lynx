// Package htn implements a hierarchical-task-network planner.
//
// HTN reasons over **tasks** rather than world-state effects directly.
// A [Task] is either:
//
//   - **primitive** — wraps one [core.Action]; emitted into the plan
//     as-is.
//   - **compound** — has a list of [Method]s; each method is a
//     decomposition recipe (preconditions + an ordered list of
//     subtask names). The planner tries methods in order; the first
//     whose preconditions match the current state and whose subtasks
//     all decompose successfully wins.
//
// The HTN planner is the right pick when:
//
//   - the domain has a clear "way to do X" hierarchy (cooking
//     recipes, build pipelines, multi-step tutorials)
//   - method selection itself encodes domain expertise that's
//     awkward to express as A*-friendly preconditions/effects
//   - you want bounded search depth — HTN runs O(method × subtask)
//     instead of A*'s exponential state-space exploration.
//
// The plan emitted is the linearised flat action sequence — the
// runtime executes it the same way it executes any GOAP-produced plan.
//
// Library construction: callers build a [Library] of named tasks at
// platform setup, then pass it to [NewPlanner]. The planner's
// PlanToGoal looks up the task whose name matches goal.Name; goals
// without a matching task return (nil, nil).
package htn

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

const defaultMaxRecursion = 64

// Task is one node in the HTN hierarchy. A task is either primitive
// (Action set, Methods empty) or compound (Methods set, Action nil).
// Mixing both fields is rejected by [Library.Add].
type Task struct {
	// Name is the unique identifier — methods reference subtasks by
	// this name, and the planner's PlanToGoal matches on goal.Name.
	Name string

	// Action is the single primitive emitted into the plan when this
	// task is reached. Set this XOR Methods.
	Action core.Action

	// Methods is the ordered list of decomposition recipes for a
	// compound task. The first applicable method wins; on subtask
	// failure the planner falls back to the next method.
	Methods []Method
}

// Method is one decomposition recipe for a compound task. The
// planner picks the first method whose Preconditions match the
// current world state and whose Subtasks all decompose.
type Method struct {
	// Name is purely descriptive — useful for traces / debugging.
	Name string

	// Preconditions guard method applicability. The state must
	// satisfy every entry for the method to be considered.
	Preconditions core.EffectSpec

	// Subtasks names the tasks this method expands into, in
	// execution order. Each name must resolve in the [Library].
	Subtasks []string
}

// IsPrimitive reports whether this task wraps a single core.Action.
func (t *Task) IsPrimitive() bool { return t.Action != nil }

// Library is the registry of named tasks the planner reasons over.
// Build one at platform setup and pass it to [NewPlanner].
type Library struct {
	tasks map[string]*Task
}

// NewLibrary returns an empty library.
func NewLibrary() *Library { return &Library{tasks: map[string]*Task{}} }

// Add registers task. Returns an error on empty name, duplicate
// name, or both Action and Methods set / both unset.
func (l *Library) Add(t *Task) error {
	if t == nil {
		return errors.New("htn library: task must not be nil")
	}
	if t.Name == "" {
		return errors.New("htn library: task name must not be empty")
	}
	if t.Action != nil && len(t.Methods) > 0 {
		return fmt.Errorf("htn library: task %q has both Action and Methods", t.Name)
	}
	if t.Action == nil && len(t.Methods) == 0 {
		return fmt.Errorf("htn library: task %q has neither Action nor Methods", t.Name)
	}
	if _, dup := l.tasks[t.Name]; dup {
		return fmt.Errorf("htn library: duplicate task name %q", t.Name)
	}
	l.tasks[t.Name] = t
	return nil
}

// MustAdd is the panicking variant of [Library.Add] — convenient at
// platform-init time where any error is a programming bug.
func (l *Library) MustAdd(t *Task) {
	if err := l.Add(t); err != nil {
		panic(err)
	}
}

// Lookup returns the task registered under name, or (nil, false).
func (l *Library) Lookup(name string) (*Task, bool) {
	t, ok := l.tasks[name]
	return t, ok
}

// Planner is the concrete HTN planner. Library is supplied at
// construction; the planner is otherwise stateless and safe to share
// across goroutines.
type Planner struct {
	library      *Library
	maxRecursion int
}

// NewPlanner returns an HTN planner backed by library. maxRecursion
// caps the decomposition depth to guard against cyclic task graphs.
func NewPlanner(library *Library) *Planner {
	if library == nil {
		panic("htn.NewPlanner: library must not be nil")
	}
	return &Planner{library: library, maxRecursion: defaultMaxRecursion}
}

// PlanToGoal decomposes the task whose name matches goal.Name. Goals
// without a matching task return (nil, nil) so the runtime can fall
// through to a different planner if registered.
//
// Errors are reserved for *structural* problems — exceeded recursion
// depth or a method referencing an unknown subtask name. Soft
// failures (a method's preconditions don't hold; a subtask's body
// has no applicable method) cause the planner to backtrack to the
// next sibling method, returning (nil, nil) only when every option
// is exhausted.
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	goal *core.Goal,
	options plan.PlanOptions,
) (*plan.Plan, error) {
	if start == nil {
		return nil, errors.New("plan to goal: start world state is nil")
	}
	if goal == nil {
		return nil, errors.New("plan to goal: goal is nil")
	}
	if system == nil {
		return nil, errors.New("plan to goal: planning system is nil")
	}

	root, ok := p.library.Lookup(goal.Name)
	if !ok {
		return nil, nil
	}

	actions, _, ok, err := p.decompose(ctx, root, start, options.ExcludedActions, 0)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &plan.Plan{Actions: actions, Goal: goal}, nil
}

// PlansToGoals enumerates plans for every goal in the system, sorted
// by NetValue descending. Goals without a matching task in the
// library are silently skipped.
func (p *Planner) PlansToGoals(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	options plan.PlanOptions,
) ([]*plan.Plan, error) {
	if system == nil {
		return nil, errors.New("plans to goals: planning system is nil")
	}
	out := make([]*plan.Plan, 0, len(system.Goals))
	for _, goal := range system.Goals {
		pl, err := p.PlanToGoal(ctx, start, system, goal, options)
		if err != nil {
			return nil, err
		}
		if pl == nil {
			continue
		}
		out = append(out, pl)
	}
	plan.SortByNetValueDesc(out, start)
	return out, nil
}

// BestValuePlan returns the highest-NetValue plan across every goal.
func (p *Planner) BestValuePlan(
	ctx context.Context,
	start core.WorldState,
	system *plan.PlanningSystem,
	options plan.PlanOptions,
) (*plan.Plan, error) {
	return plan.BestOf(p.PlansToGoals(ctx, start, system, options))
}

// Prune is a no-op — HTN's library is already pre-curated; pruning
// at planner level would invalidate user-supplied tasks.
func (p *Planner) Prune(system *plan.PlanningSystem) *plan.PlanningSystem {
	return system
}

// decompose recursively expands task into a flat action list,
// threading the world state through so each subtask sees the effects
// of its predecessors. Returns:
//
//   - (actions, finalState, true, nil) on success;
//   - (nil, state, false, nil) when no method applies — the caller (a
//     parent method) treats this as a backtrack signal and tries its
//     next method;
//   - (nil, _, _, err) on a structural error that aborts the entire
//     plan (recursion depth exceeded; method references an unknown
//     subtask; ctx cancelled).
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
		return nil, state, false, fmt.Errorf("htn: exceeded max recursion depth %d at task %q", p.maxRecursion, task.Name)
	}

	if task.IsPrimitive() {
		meta := task.Action.Metadata()
		if _, skip := excluded[meta.Name]; skip {
			return nil, state, false, nil
		}
		return []core.Action{task.Action}, state.Apply(meta.Effects), true, nil
	}

	// Snapshot once so all method-applicability probes share the same
	// view (and we don't pay one defensive map copy per method).
	stateMap := state.State()

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
			return nil, state, false, fmt.Errorf("htn: method %q references unknown subtask %q", method.Name, subtaskName)
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
func methodApplicable(method Method, state map[string]core.Determination) bool {
	for key, required := range method.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}
