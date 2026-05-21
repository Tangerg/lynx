// Package runtime is the agent runtime — it owns the [Platform] that
// holds deployed agents, builds [AgentProcess] instances, drives the
// plan / act / observe / repeat loop, and wires every plug-in point.
//
// The runtime's only configuration mechanism is the [core.Extension]
// registry. Every cross-cutting concern — event listeners, action
// interceptors, tool decorators, agent validators, goal approvers,
// tool-group resolvers, id generators, blackboard factories, planner
// factories — is an Extension that the Platform discovers by type
// assertion at dispatch time. Per-process Extensions merge with the
// Platform-scoped set when a process is created, so per-call
// overrides remain idiomatic.
//
// Process lifecycle:
//
//	NewPlatform → Deploy(agent) → Run(processID, opts) → tick loop
//	  → Resume(processID, hitlResult)  // for HITL flows
//	  → Kill / Remove / Prune          // termination + cleanup
//
// HITL is a first-class state: when an action surfaces an [hitl]
// awaitable, the process suspends and Resume re-enters from the same
// blackboard state with the operator's reply folded in. Concurrent
// child processes (via [Platform.CreateChildProcess]) inherit the
// parent's chat client + extensions but get their own blackboard.
//
// OTel: every action invocation, planner replan, and platform Run
// produces a span under the `lynx/agent` tracer. See `agent/tracing`
// in source for the attribute schema.
package runtime
