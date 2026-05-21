package htn

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

// plannerTracer is the package-level tracer for the HTN planner.
// Tracer name follows the `lynx/agent/planner` namespace shared with
// the GOAP A* planner — backends can distinguish algorithms by the
// span name (`htn.plan` vs `goap.astar`).
var plannerTracer = otel.Tracer("lynx/agent/planner")

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
		return errors.New("htn.Library.Add: task must not be nil")
	}
	if t.Name == "" {
		return errors.New("htn.Library.Add: task name must not be empty")
	}
	if t.Action != nil && len(t.Methods) > 0 {
		return fmt.Errorf("htn.Library.Add: task %q has both Action and Methods", t.Name)
	}
	if t.Action == nil && len(t.Methods) == 0 {
		return fmt.Errorf("htn.Library.Add: task %q has neither Action nor Methods", t.Name)
	}
	if _, dup := l.tasks[t.Name]; dup {
		return fmt.Errorf("htn.Library.Add: duplicate task name %q", t.Name)
	}
	l.tasks[t.Name] = t
	return nil
}

// MustAdd is the panicking variant of [Library.Add] — convenient at
// platform-init time where any error is a programming bug. Panic is
// intentional here (the "Must" prefix is the Go convention for that).
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
// Returns an error when library is nil.
func NewPlanner(library *Library) (*Planner, error) {
	if library == nil {
		return nil, fmt.Errorf("htn.NewPlanner: library must not be nil")
	}
	return &Planner{library: library, maxRecursion: defaultMaxRecursion}, nil
}

// Name is the planner's extension identifier — the value an agent's
// [core.AgentConfig.PlannerName] must match to select this planner.
func (p *Planner) Name() string { return "htn" }

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
) (result *plan.Plan, err error) {
	if err = plan.CheckPlanInputs(start, system, goal); err != nil {
		return nil, err
	}

	ctx, span := plannerTracer.Start(ctx, "htn.plan",
		trace.WithAttributes(
			attribute.String("lynx.agent.planner", "htn"),
			attribute.String("lynx.agent.goal.name", goal.Name),
		),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if result != nil {
			span.SetAttributes(attribute.Int("lynx.agent.plan.length", len(result.Actions)))
		}
		span.End()
	}()

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
	result = &plan.Plan{Actions: actions, Goal: goal}
	return result, nil
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
		return nil, state, false, fmt.Errorf("htn.Planner.decompose: exceeded max recursion depth %d at task %q", p.maxRecursion, task.Name)
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
func methodApplicable(method Method, state map[string]core.Determination) bool {
	for key, required := range method.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}
