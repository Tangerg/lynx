// Package goap implements the default goal-oriented action planner. It uses
// deterministic uniform-cost search (A* with h=0), which finds a cheapest plan
// for finite non-negative action costs without assuming domain-specific cost
// lower bounds. It preserves the search path exactly: ScoreFunc may depend on
// any world-state condition, so relevance pruning or post-search action removal
// would not be generally cost-safe.
package goap
