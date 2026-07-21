// Package agent is the standard façade for the Lynx Agent Framework. It
// exposes definition, deployment, execution, status, session, interaction, and
// suspension types used by the common lifecycle. These are aliases to their
// owning core/runtime/interaction types, not copied abstractions.
//
// Advanced protocols remain in focused sub-packages: custom planners,
// Blackboard/store implementations, event payloads, tool-loop internals,
// workflow builders, and provider/tool adapters are imported only when used.
//
// Sub-package map:
//
//	github.com/Tangerg/lynx/agent/core        — primitives (Action / Goal / Condition / Agent / Blackboard / status enums)
//	github.com/Tangerg/lynx/agent/planning    — Plan / Planner interface / Domain
//	github.com/Tangerg/lynx/agent/planning/goap
//	github.com/Tangerg/lynx/agent/planning/htn
//	github.com/Tangerg/lynx/agent/routing     — prompt-to-agent routing
//	github.com/Tangerg/lynx/agent/runtime     — Engine / Process
//	github.com/Tangerg/lynx/agent/event       — event types + listener
//	github.com/Tangerg/lynx/agent/workflow    — scatter-gather / repeat-until agent builders
//	github.com/Tangerg/lynx/agent/toolpolicy  — chat-tool policies (once-only / unlock)
//	github.com/Tangerg/lynx/agent/interaction — managed interaction and suspension protocol
//	github.com/Tangerg/lynx/agent/hitl        — linear typed Interrupt[R]
package agent
