// Package service holds the transport-agnostic Go interfaces that form
// Lyra's stable runtime surface. Every transport adapter — HTTP+SSE,
// inprocess, future MCP — maps onto these interfaces; the business logic
// lives in the inmemory.go / engine.go files under each sub-package, the
// wire formats live elsewhere.
//
// Each sub-package declares one service:
//
//   - session/   — session lifecycle (list/create/fork/delete/resume)
//   - chat/      — one-turn dispatch + event stream
//   - tool/      — tool registry inspection and (optional) direct invocation
//   - approval/  — permission decisions for tool calls
//   - memory/    — long-term memory (LYRA.md cascade)
//   - agentdoc/  — AGENTS.md cascade discovery + render
//
// Design constraints:
//
//   - Method signatures use only Go std types (context.Context, channels,
//     plain structs). No proto/pb types ever leak into the service layer.
//   - Event types are channel-based — the natural Go idiom for streaming.
//   - Concrete implementations sit alongside (inmemory.go for the in-process
//     impls, engine.go where the impl is engine-backed), built on lynx-agent
//     and lynx-core.
//
// Transport layer (M8+) wraps these interfaces; the interfaces themselves
// never change between Phase 1 (in-process Go API) and Phase 2 (multi-
// transport). See lyra/doc/ARCHITECTURE.md §2.1.
package service
