// Package service holds the six transport-agnostic Go interfaces that
// form Lyra's stable runtime surface. Every transport adapter — HTTP+SSE,
// gRPC, stdio JSON-RPC, MCP — maps onto these interfaces; the business
// logic lives in the *impl.go files under each sub-package, the wire
// formats live elsewhere.
//
// Each sub-package declares one service:
//
//   - session/   — session lifecycle (list/create/fork/delete/resume)
//   - chat/      — one-turn dispatch + event stream
//   - tool/      — tool registry inspection and (optional) direct invocation
//   - approval/  — permission decisions for tool calls
//   - memory/    — long-term memory (LYRA.md cascade)
//   - trace/     — observability (span tree + live span stream)
//
// Design constraints:
//
//   - Method signatures use only Go std types (context.Context, channels,
//     plain structs). No proto/pb types ever leak into the service layer.
//   - Event types are channel-based — the natural Go idiom for streaming.
//   - Concrete implementations sit alongside (impl.go), backed by lynx-agent
//     and lynx-core.
//
// Transport layer (M8+) wraps these interfaces; the interfaces themselves
// never change between Phase 1 (in-process Go API) and Phase 2 (multi-
// transport). See lyra/doc/ARCHITECTURE.md §2.1.
package service
