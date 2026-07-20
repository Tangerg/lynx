package htn

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

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
func (m Method) applicable(state core.ConditionSet) bool {
	return state.Satisfies(m.Preconditions)
}

// IsPrimitive reports whether this task wraps a single core.Action.
func (t *Task) IsPrimitive() bool { return t.Action != nil }

// Library is the registry of named tasks the planner reasons over.
type Library struct {
	mu    sync.RWMutex
	tasks map[string]Task
}

// NewLibrary returns an empty library.
func NewLibrary() *Library { return &Library{} }

// Add snapshots task and rejects malformed names, methods, or task shapes.
func (l *Library) Add(t *Task) error {
	if l == nil {
		return errors.New("htn.Library.Add: library must not be nil")
	}
	if t == nil {
		return errors.New("htn.Library.Add: task must not be nil")
	}
	if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Name) != t.Name {
		return fmt.Errorf("htn.Library.Add: task name %q must be non-empty without surrounding whitespace", t.Name)
	}
	if t.Action != nil && len(t.Methods) > 0 {
		return fmt.Errorf("htn.Library.Add: task %q has both Action and Methods", t.Name)
	}
	if t.Action == nil && len(t.Methods) == 0 {
		return fmt.Errorf("htn.Library.Add: task %q has neither Action nor Methods", t.Name)
	}
	for index, method := range t.Methods {
		if err := method.Preconditions.Validate(); err != nil {
			return fmt.Errorf("htn.Library.Add: task %q method[%d] preconditions: %w", t.Name, index, err)
		}
		if len(method.Subtasks) == 0 {
			return fmt.Errorf("htn.Library.Add: task %q method[%d] has no subtasks", t.Name, index)
		}
		for subtaskIndex, subtask := range method.Subtasks {
			if strings.TrimSpace(subtask) == "" || strings.TrimSpace(subtask) != subtask {
				return fmt.Errorf("htn.Library.Add: task %q method[%d] subtask[%d] %q must be non-empty without surrounding whitespace", t.Name, index, subtaskIndex, subtask)
			}
		}
	}

	cloned := cloneTask(*t)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tasks == nil {
		l.tasks = make(map[string]Task)
	}
	if _, duplicate := l.tasks[t.Name]; duplicate {
		return fmt.Errorf("htn.Library.Add: duplicate task name %q", t.Name)
	}
	l.tasks[t.Name] = cloned
	return nil
}

// MustAdd is the panicking variant of [Library.Add] for static setup.
func (l *Library) MustAdd(t *Task) {
	if err := l.Add(t); err != nil {
		panic(err)
	}
}

func (l *Library) Lookup(name string) (*Task, bool) {
	if l == nil {
		return nil, false
	}
	l.mu.RLock()
	task, ok := l.tasks[name]
	l.mu.RUnlock()
	if !ok {
		return nil, false
	}
	cloned := cloneTask(task)
	return &cloned, true
}

func (l *Library) snapshot() map[string]Task {
	l.mu.RLock()
	defer l.mu.RUnlock()
	tasks := make(map[string]Task, len(l.tasks))
	for name, task := range l.tasks {
		tasks[name] = cloneTask(task)
	}
	return tasks
}

func cloneTask(task Task) Task {
	task.Methods = slices.Clone(task.Methods)
	for index := range task.Methods {
		task.Methods[index].Preconditions = maps.Clone(task.Methods[index].Preconditions)
		task.Methods[index].Subtasks = slices.Clone(task.Methods[index].Subtasks)
	}
	return task
}
