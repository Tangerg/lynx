// Package planning provides goal-directed state planning as an Agent execution
// strategy. Goal, Condition, WorldState, Action, and Plan belong exclusively to
// this package; the Agent kernel sees only opaque Execution state and Effects.
//
// Planning separates predicted Action semantics from external execution. A
// Planner is a pure, deterministic search over a Problem. A managed Planning
// Execution observes the real world and executes selected Actions outside its
// Step through a Deployment-bound dispatcher or a child Process, then observes
// again before accepting that the prediction became true.
package planning
