// Package runtime is the agent runtime — it owns the [Engine] that
// holds deployed agents, builds [Process] instances, drives the
// plan / act / observe / repeat loop, and wires every plug-in point.
//
// The runtime's behavioral plug-in mechanism is the [core.Extension]
// registry. Cross-cutting concerns — event listeners, action
// and tool middleware, agent validators, goal approvers,
// tool-group resolvers, id generators, blackboard prototypes, and planners
// — are Extensions that the Engine discovers by type
// assertion at dispatch time. Per-process Extensions merge with the
// Engine-scoped set when a process is created, so per-call
// overrides remain idiomatic. Stable construction dependencies such as chat,
// ProcessStore, SessionStore, and snapshot policy remain explicit fields on
// [Config]; they are not hidden in the extension registry.
//
// Process lifecycle:
//
//	New → Deploy(agent) → immutable Deployment
//	  → Run(ctx, agent, bindings, options)             // synchronous run
//	  → Start / RunInSession                           // background / multi-turn variants
//	  → Resume(id, suspensionID, response) + Continue // record reply, re-enter loop
//	  → Kill / Remove / Prune
//
// HITL is a first-class state: when an action surfaces a suspension from
// [hitl.Interrupt],
// suspension, the process waits in [core.StatusWaiting];
// [Engine.Resume] records a response on the exact suspension while
// the process remains waiting; [Engine.Continue] re-enters the action
// at that suspension point. [Engine.RunChildWithState], [Engine.RunChild], and
// [Engine.RunChildIsolated] bind an exact Deployment with explicit inheritance
// semantics, join the parent's budget tree, and receive
// its process-scope [EventListener] extensions. Other process extensions,
// guardrails, and dependency overrides remain scoped to the declaring process.
//
// OTel: every action invocation, planner replan, and engine run
// produces a span under the `lynx/agent` tracer (planners use
// `lynx/agent/planner`). See doc/OBSERVABILITY.md for the attribute
// schema.
package runtime
