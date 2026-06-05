// Package agent is the single user-facing surface for the Lynx agent
// framework. Following the design of std packages like net/http and the
// MCP Go SDK's mcp package, the types, constants, and constructors a
// caller needs to define, run, extend, and persist an agent are all
// reachable here — promoted (via aliases in aliases.go and thin wrappers
// alongside the [Builder]) from the framework's internal layering:
//
//	core    — primitives: Action / Goal / Condition / Agent / Blackboard /
//	          ProcessContext / Extension points / status enums
//	runtime — engine: Platform / AgentProcess / snapshot+restore
//
// So the common case imports only this package:
//
//	p := agent.NewPlatform(agent.PlatformConfig{})
//	def := agent.New("greeter").
//		Actions(agent.NewAction("greet", greet, agent.ActionConfig{})).
//		Goals(agent.GoalProducing[Reply](core.Goal{Description: "reply"})).
//		Build()
//	// where greet is func(ctx, pc *agent.ProcessContext, in Req) (Reply, error)
//
// The promotions are zero-cost type aliases — agent.Action IS core.Action.
// The core / runtime packages stay importable directly for advanced use,
// and consumers remain free to depend on their own narrow interfaces over
// these types (the framework promotes the concrete types for
// discoverability, not to dictate how callers couple to them).
//
// Self-contained features keep their own packages and are imported when
// used:
//
//	github.com/Tangerg/lynx/agent/planning   — Planner interface + concrete planners (goap, htn, …)
//	github.com/Tangerg/lynx/agent/event      — lifecycle events + multicast listener
//	github.com/Tangerg/lynx/agent/workflow   — Sequence / Loop / Parallel / Consensus builders
//	github.com/Tangerg/lynx/agent/hitl       — typed Awaitable / NewConfirmation
//	github.com/Tangerg/lynx/agent/toolpolicy — chat-tool decorators (once-only / unlock)
package agent
