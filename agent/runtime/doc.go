// Package runtime is the agent runtime — it owns the [Engine] that
// holds deployed agents, builds [Process] instances, drives the
// plan / act / observe / repeat loop, and wires every plug-in point.
//
// Behavioral plug-ins all arrive through one mechanism: a [core.Extension]
// whose capabilities the Engine discovers by type assertion at dispatch time.
// See [Config.Extensions] for the capabilities that mechanism carries.
// Per-process Extensions merge with the Engine-scoped set at process creation,
// so a per-call override needs no separate seam. Execution dependencies that
// every run needs — chat, chat middleware, Prompt limits, child depth — stay
// explicit fields on [Config] instead, so a missing one is a construction error
// rather than a silently absent extension. Product data and identity stay
// outside this package; runtime only captures and rebuilds portable process
// values.
//
// Process lifecycle:
//
//	New → Deploy(agent) → immutable Deployment
//	  → Run(ctx, agent, bindings, options)             // synchronous run
//	  → Start                                          // background segment
//	  → Resume(ctx, id, suspensionID, response) + Continue // record reply, re-enter loop
//	  → ResumeAsync(admissionCtx, runCtx, ...)          // atomically reply + own a Segment
//	  → PendingSuspensions                              // direct external waits across the tree
//	  → SnapshotTree / RestoreTree                      // portable complete-tree state, no I/O
//	  → PrepareWaitingSubtreeCancellation               // caller-coordinated waiting child cancel
//	  → Kill / RemoveTree
//
// HITL is a first-class state: when an action surfaces a suspension from
// [hitl.Interrupt], the process waits in [core.StatusWaiting];
// [Engine.Resume] records a response on the exact suspension while
// the process remains waiting; [Engine.Continue] re-enters the action
// at that suspension point. [Engine.ResumeAsync] combines those two admission
// transitions atomically for asynchronous hosts. [Engine.PendingSuspensions]
// gives a coordinating host the complete, source-attributed set of unanswered
// boundaries without exposing framework checkpoints. A synchronous AgentTool
// child that waits promotes the same suspension to its parent and retains the
// exact child/tool-loop checkpoint, so Resume/Continue finishes the original
// tool call without replaying completed siblings.
// [Engine.PrepareWaitingSubtreeCancellation] freezes that complete tree while
// its caller coordinates the replacement with external state; Commit then
// applies the prevalidated runtime mutation, while Abort leaves live state
// unchanged.
// [Engine.RunChildWithState] and [Engine.RunChild]
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
