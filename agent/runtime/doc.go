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
// overrides remain idiomatic. Stable execution dependencies such as chat,
// chat middleware, Prompt limits, and child-depth limits remain explicit
// fields on [Config]; they are not hidden in the extension registry. Product data management and
// identity stay outside this package; runtime only captures and rebuilds
// portable process values.
//
// Process lifecycle:
//
//	New → Deploy(agent) → immutable Deployment
//	  → Run(ctx, agent, bindings, options)             // synchronous run
//	  → Start                                          // background segment
//	  → Resume(ctx, id, suspensionID, response) + Continue // record reply, re-enter loop
//	  → ResumeAsync(admissionCtx, runCtx, ...)          // atomically reply + own a Segment
//	  → SnapshotTree / RestoreTree                      // portable complete-tree state, no I/O
//	  → Kill / RemoveTree
//
// HITL is a first-class state: when an action surfaces a suspension from
// [hitl.Interrupt], the process waits in [core.StatusWaiting];
// [Engine.Resume] records a response on the exact suspension while
// the process remains waiting; [Engine.Continue] re-enters the action
// at that suspension point. [Engine.ResumeAsync] combines those two admission
// transitions atomically for asynchronous hosts. A synchronous AgentTool child that waits promotes
// the same suspension to its parent and retains the exact child/tool-loop
// checkpoint, so Resume/Continue finishes the original tool call without
// replaying completed siblings. [Engine.RunChildWithState] and [Engine.RunChild]
// bind an exact Deployment with explicit inheritance
// semantics, join the parent's budget tree, and receive
// its process-scope [SubtreeEventListener] extensions. Other process extensions,
// chat middleware and dependency overrides remain scoped to the declaring process.
//
// OTel: every action invocation, planner replan, and engine run
// produces a span under the `lynx/agent` tracer (planners use
// `lynx/agent/planner`). See doc/OBSERVABILITY.md for the attribute
// schema.
package runtime
