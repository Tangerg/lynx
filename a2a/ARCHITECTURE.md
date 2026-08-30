# a2a architecture

> A thin Scope adapter over the official Agent-to-Agent SDK. The protocol wire,
> AgentCard, transport, and task lifecycle belong to the SDK; this module adds
> only the Scope-facing edges.

Repository-wide rules live in [`../CLAUDE.md`](../CLAUDE.md). Symbols and SDK
versions follow the code; the usage entry point is [`README.md`](README.md).

---

## 1. Position

- **The root package holds the A2A helpers and the tool adapter**: resolving an
  AgentCard, opening a client, projecting content, the server executor, and
  folding a remote agent into a `tool.Tool`.
- **Both directions.** A remote A2A agent becomes a locally callable tool, and a
  Scope text-streaming capability becomes an A2A endpoint.

## 2. Mental model

- **Thin adaptation, no second protocol state.** The JSON-RPC envelope, SSE, and
  the AgentCard schema all come from the official SDK. Nothing is re-declared.
- **A remote agent is exposed only as a tool view.** `OpenToolSet` opens clients
  and resolves agents in one batch; the returned `ToolSet` owns both an
  immutable `[]tool.Tool` view and an idempotent close. The SDK client is not
  exposed, and there is no provider, cache, or registry.
- **An A2A tool takes one message field.** A2A is a message protocol, not a
  typed function call, and the tool surface does not pretend otherwise.
- **Content projection is text-first**, mapping A2A content into Scope's
  text-first semantics.
- **The executor's event sequence must be legal**: submitted → working →
  incremental output → completed, failed, or canceled. An empty increment is
  skipped, because the SDK rejects an empty artifact.

## 3. Observability

As a protocol integration, this module may use the official OpenTelemetry API
directly at the A2A call boundary it owns. That exemption does not spread: no
observability type may leak into an Agent or Core contract.

## 4. Negative invariants

- Never write protocol state here — JSON-RPC, SSE, or the AgentCard schema.
  Use the SDK.
- Never add a provider, cache, or registry. `ToolSet` owns one batch connection's
  tool view and its release; it carries no discovery, refresh, or caching policy.
- Never split the package by subdomain. The whole A2A adaptation domain starts in
  the root package.

## 5. Read before changing

Start from the official A2A SDK's interface shapes. This module holds no
protocol state of its own, so a change here is usually a change in how an SDK
type is projected.
