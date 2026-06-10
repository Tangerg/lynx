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
//	NewPlatform → Deploy(agent)
//	  → RunAgent(ctx, agentDef, bindings, options)    // synchronous run
//	  → StartAgent / RunInSession                     // background / multi-turn variants
//	  → ResumeProcess(id, response) + ContinueProcess // HITL: deliver reply, re-enter loop
//	  → KillProcess / RemoveProcess / PruneTerminalProcesses
//
// HITL is a first-class state: when an action surfaces an [hitl]
// awaitable, the process suspends in [core.StatusWaiting];
// [Platform.ResumeProcess] folds the operator's reply into the
// blackboard and [Platform.ContinueProcess] re-enters the loop from
// that state. Child processes (via [Platform.CreateChildProcess])
// inherit the parent's blackboard via Spawn (unless overridden) and
// its process-scope [EventListener] extensions — other extensions
// stay scoped to the process that declared them.
//
// OTel: every action invocation, planner replan, and platform run
// produces a span under the `lynx/agent` tracer (planners use
// `lynx/agent/planner`). See doc/OBSERVABILITY.md for the attribute
// schema.
package runtime
