// Package reactive implements a one-step utility-scoring planner.
//
// Where GOAP searches for an optimal action sequence, the reactive
// planner picks just the *next* action — the one whose effects close
// the most goal preconditions, with low cost as a tie-breaker. The
// resulting [planning.Plan] always has at most one action; the runtime
// drives the agent toward the goal by replanning every tick.
//
// This is the right planner when:
//
//   - the world changes between ticks (event-driven domains where
//     a multi-step plan would be stale by the time the second action
//     runs)
//   - actions are inherently incremental (chat agents, monitoring
//     loops) and the goal is "make progress, then re-evaluate"
//   - the action space is too large for A* but you still want goal-
//     directed behaviour
//
// Mirrors embabel's UtilityPlanner — same shape, different name.
package reactive
