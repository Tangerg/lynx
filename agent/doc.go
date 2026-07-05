// Package agent is the convenience surface for the Lynx agent framework.
// It exposes the fluent [Builder] plus the small set of constructors a
// typical caller reaches for — everything else (types, constants,
// helpers) lives in the sub-packages and must be imported explicitly.
//
// Sub-package map:
//
//	github.com/Tangerg/lynx/agent/core        — primitives (Action / Goal / Condition / Agent / Blackboard / status enums)
//	github.com/Tangerg/lynx/agent/planning        — Plan / Planner interface / planning.System / concrete planners (goap, …)
//	github.com/Tangerg/lynx/agent/runtime     — Platform / AgentProcess
//	github.com/Tangerg/lynx/agent/event       — event types + listener
//	github.com/Tangerg/lynx/agent/workflow    — scatter-gather / repeat-until agent builders
//	github.com/Tangerg/lynx/agent/toolpolicy  — chat-tool decorators (once-only / unlock)
//	github.com/Tangerg/lynx/agent/hitl        — Interrupt[R] + typed Awaitable helpers
package agent
