// Package goap implements the A* GOAP planner — the default planner for
// the agent runtime. It searches from the current world state to a state
// that satisfies the goal's preconditions, using "number of unsatisfied
// goal conditions" as an admissible heuristic (so A* is guaranteed to find
// an optimal plan).
package goap
