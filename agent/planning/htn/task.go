package htn

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

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
	Preconditions core.ConditionSet

	// Subtasks names the tasks this method expands into, in
	// execution order. Each name must resolve in the [Library].
	Subtasks []string
}

// applicable reports whether every method precondition holds in state.
func (m Method) applicable(state map[string]core.Truth) bool {
	for key, required := range m.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}

// IsPrimitive reports whether this task wraps a single core.Action.
func (t *Task) IsPrimitive() bool { return t.Action != nil }

// Library is the registry of named tasks the planner reasons over.
type Library struct {
	tasks map[string]*Task
}

// NewLibrary returns an empty library.
func NewLibrary() *Library { return &Library{tasks: map[string]*Task{}} }

// Add registers task, rejecting empty names, duplicates, and tasks that are
// neither or both primitive and compound.
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
	if _, duplicate := l.tasks[t.Name]; duplicate {
		return fmt.Errorf("htn.Library.Add: duplicate task name %q", t.Name)
	}
	l.tasks[t.Name] = t
	return nil
}

// MustAdd is the panicking variant of [Library.Add] for static setup.
func (l *Library) MustAdd(t *Task) {
	if err := l.Add(t); err != nil {
		panic(err)
	}
}

func (l *Library) Lookup(name string) (*Task, bool) {
	task, ok := l.tasks[name]
	return task, ok
}
