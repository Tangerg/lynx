package htn

import "github.com/Tangerg/lynx/agent/core"

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

// IsPrimitive reports whether this task wraps a single core.Action.
func (t *Task) IsPrimitive() bool { return t.Action != nil }
