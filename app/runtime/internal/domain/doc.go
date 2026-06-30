// Package domain is the bounded-context layer of the Clean Arch ring: one
// sub-package per business capability, each holding entities, domain services,
// and consumer-side ports. The capabilities are composed by the kernel and
// exposed at the wire by delivery; the domain packages themselves know nothing
// of transports, wire formats, or driven adapters.
//
// Each sub-package owns one bounded context:
//
//   - session/      — session lifecycle (list/create/fork/delete/resume)
//   - knowledge/    — long-term knowledge (LYRA.md cascade)
//   - transcript/   — items + runs timeline backing items.list
//   - conversation/ — the LLM message context fed to a turn
//   - maintenance/  — compaction / extraction / planning (turn-boundary ops)
//   - approval/     — runtime tool-approval stance (Mode: pass/deny/prompt)
//   - tool/         — tool registry inspection and (optional) direct invocation
//   - editguard/    — read-before-edit + stale invariants
//   - interrupts/   — HITL interrupt store (park on interrupt, resume)
//   - provider/     — runtime LLM-provider registry (per-provider key + baseURL)
//   - skills/       — skill discovery + retrieval
//   - todo/         — model-facing task list
//   - agentdoc/     — AGENTS.md cascade discovery + render
//
// A bounded context that needs replaceable storage (session, knowledge,
// transcript, provider, interrupts, …) defines a consumer-side Service/Store
// interface and an implementation named for its essence (sqlite-backed,
// file-backed, engine-backed). See lyra/doc/GREENFIELD_ARCHITECTURE.md.
package domain
