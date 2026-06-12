// Package utility provides two value-based planners: classic utility-AI
// and a hybrid variant with goal-satisfaction termination.
//
// Both planners score every applicable action by its net value
// ([core.Action.Value] − [core.Action.Cost]) and pick the highest.
// They differ in how they decide when to stop:
//
//   - [NewPlanner] is the classic Utility AI shape: pick the best
//     action when the goal is not yet satisfied; if no available
//     action can satisfy the goal in one step, return nil.
//     A special "Nirvana" goal (unsatisfiable, [NirvanaGoalName])
//     keeps producing one-step picks forever — useful for
//     exploratory / chat surfaces.
//
//   - [NewHybridPlanner] adds goal-satisfaction termination: the
//     "is goal already satisfied?" check happens BEFORE action
//     picking, so once the real goal lands the planner returns an
//     empty plan and the process terminates. Pairs naturally with
//     Nirvana for "iterate until done" pipelines.
//
// Use [NewPlanner] for pure-iteration loops (chat, exploration);
// use [NewHybridPlanner] when combining a real terminal goal with
// opportunistic intermediate work (research-then-summarize,
// gather-then-decide pipelines).
//
// For multi-step search with cost minimization choose the
// [goap.Planner] instead; for hierarchical task decomposition use
// [htn.Planner].
package utility
