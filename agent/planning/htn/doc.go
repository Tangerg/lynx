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
//     awkward to express as flat GOAP preconditions/effects
//   - you want bounded search depth — HTN runs O(method × subtask)
//     instead of GOAP's state-space exploration.
//
// The plan emitted is the linearised flat action sequence — the
// runtime executes it the same way it executes any GOAP-produced plan.
//
// Library construction: callers build a [Library] of named tasks at
// engine setup, then pass it to [NewPlanner]. The planner's
// PlanToGoal looks up the task whose name matches goal.Name; goals
// without a matching task return (nil, nil).
package htn
