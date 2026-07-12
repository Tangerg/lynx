// Package domain is the bounded-context layer of the Clean Arch ring: one
// sub-package per business capability, each holding entities, domain services,
// and consumer-side ports. The capabilities are composed by the kernel and
// exposed at the wire by delivery; the domain packages themselves know nothing
// of transports, wire formats, or driven adapters.
//
// Each sub-package owns one bounded context:
//
//   - accounting/   — token/cost roll-ups and turn budget rules
//   - session/      — session lifecycle (list/create/fork/delete/resume)
//   - knowledge/    — long-term knowledge (LYRA.md cascade)
//   - transcript/   — items + runs timeline backing items.list
//   - conversation/ — the LLM message context fed to a turn
//   - approval/     — runtime tool-approval stance (Mode: pass/deny/prompt)
//   - tool/         — tool registry inspection and (optional) direct invocation
//   - editguard/    — read-before-edit + stale invariants
//   - interrupts/   — HITL interrupt store (park on interrupt, resume)
//   - provider/     — runtime LLM-provider registry (per-provider key + baseURL)
//   - modelrole/    — provider/model assignments for specialized roles
//   - mcpserver/    — MCP registry entries and effective tool policy
//   - skills/       — skill discovery + retrieval
//   - todo/         — model-facing task list
//   - agentdoc/     — AGENTS.md cascade discovery + render
//
// A bounded context that needs replaceable storage or policy evaluation
// (session, knowledge, transcript, provider, interrupts, approval, …) defines a
// consumer-side Store / Registry / Policy interface and an implementation named
// for its essence (sqlite-backed, file-backed, engine-backed). See
// lyra/doc/EXECUTION_CENTERED_ARCHITECTURE.md.
package domain
