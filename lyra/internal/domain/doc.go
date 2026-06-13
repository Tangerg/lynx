// Package service holds the transport-agnostic Go interfaces that form
// Lyra's stable runtime surface. Every transport adapter — HTTP+SSE,
// inprocess, future MCP — maps onto these interfaces; the business logic
// lives in the inmemory.go / engine.go files under each sub-package, the
// wire formats live elsewhere.
//
// Each sub-package declares one domain service:
//
//   - session/    — session lifecycle (list/create/fork/delete/resume)
//   - chat/       — one-turn dispatch + event stream
//   - tool/       — tool registry inspection and (optional) direct invocation
//   - approval/   — runtime tool-approval stance (Mode: pass/deny/prompt)
//   - knowledge/  — long-term knowledge (LYRA.md cascade)
//   - agentdoc/   — AGENTS.md cascade discovery + render
//   - history/    — authoritative Item store backing items.list
//   - interrupts/ — HITL interrupt store (park on interrupt, resume)
//   - provider/   — runtime LLM-provider registry (per-provider key + baseURL)
//
// Design constraints:
//
//   - Method signatures use only Go std types (context.Context, channels,
//     plain structs). No proto/pb types ever leak into the domain layer.
//   - Event types are channel-based — the natural Go idiom for streaming.
//   - Concrete implementations sit alongside (inmemory.go for the in-process
//     impls, engine.go where the impl is engine-backed), built on lynx-agent
//     and lynx-core.
//
// The transport layer wraps these interfaces; the interfaces stay
// transport-agnostic. See lyra/doc/GREENFIELD_ARCHITECTURE.md
package service
