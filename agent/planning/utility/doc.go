// Package utility provides two value-based planners: classic utility-AI
// and a goal-first variant with goal-satisfaction termination.
//
// Both planners score every applicable action by its net value
// ([core.ActionMetadata.Value] − [core.ActionMetadata.Cost]) and pick the highest.
// They differ in how they decide when to stop:
//
//   - [NewPlanner] is the classic Utility AI shape: pick the best
//     action and return it when its effects satisfy the goal; if no
//     available action can satisfy the goal in one step, return nil.
//
//   - [NewGoalFirst] adds goal-satisfaction termination: the
//     "is goal already satisfied?" check happens BEFORE action
//     picking, so once the goal lands the planner returns an empty
//     plan and the process terminates.
//
// Use [NewPlanner] when the top-ranked action should still run against an
// already-satisfied goal; use [NewGoalFirst] when satisfaction must terminate
// immediately.
//
// For multi-step search with cost minimization choose the
// [goap.Planner] instead; for hierarchical task decomposition use
// [htn.Planner].
package utility
