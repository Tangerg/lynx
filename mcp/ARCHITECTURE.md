# mcp architecture

> A thin Scope adapter over the official Model Context Protocol Go SDK. Client,
> server, session, and transport come from the SDK; this module adds only the
> Scope-facing edges.

Repository-wide rules live in [`../AGENTS.md`](../AGENTS.md). The usage entry
point is [`README.md`](README.md); the package-organization rationale is in
[`DESIGN.md`](DESIGN.md).

---

## 1. Position

- **The root package holds the MCP edges Scope needs**: context `_meta`,
  server-to-client reverse capabilities, the two-way adaptation between
  `core/tool.Tool` and an MCP tool, and prompt conversion.
- **Host concerns stay out**: an MCP server inventory, OAuth login, reconnect
  policy, and status display belong to a Host runtime. This module adapts the
  protocol and nothing else.

## 2. Mental model

- **Thin adapter, not a wrapper SDK.** Transports and sessions are used as the
  official types. Nothing here re-declares a protocol primitive.
- **One package, small surface.** The whole MCP adaptation domain lives in the
  root package. Remote tools are published only as `[]tool.Tool` through
  `DiscoverTools`; the concrete wrapper stays unexported so it can change
  without breaking a consumer.
- **No provider or cache layer.** Refresh policy belongs to the caller: after a
  `list-changed` notification the caller lists again. A cache here would have to
  guess an invalidation rule the protocol deliberately leaves open.
- **Protocol errors and tool errors are different failures.** A remote tool that
  reports an error is projected to a tool-level error; a transport or protocol
  failure stays a wrapped Go error. In the server direction the inverse holds: a
  `tool.Tool` failure becomes a result marked `IsError`, never a JSON-RPC error.
- **Optional capabilities travel by assertion.** `MCPToolIdentity` and
  `ConcurrencyKey` are discovered on the wrapper rather than added to the
  `tool.Tool` contract, because a new interface method would break every
  external implementation.
- **Content mapping is total.** Every MCP content shape maps to a `chat.Part`;
  an unknown or unusable shape is preserved as encoded text rather than silently
  dropped, so a caller can still see what the server sent.

## 3. Observability

As a protocol integration, this module may use the official OpenTelemetry API
directly at the MCP call boundary it owns. That exemption does not spread: no
observability type may leak into `tool.Tool` or any Core contract.

## 4. Negative invariants

- Never add a configuration registry — server lists, OAuth handlers, headers,
  reconnect settings all belong to a Host.
- Never reintroduce a provider or cache layer unless several real callers prove
  that caller-side refresh is insufficient.
- Never wrap an MCP primitive into a framework. Prefer exposing the SDK type or
  writing one small function.
- Never promote a wrapper capability into the `tool.Tool` interface.

## 5. Read before changing

- Start from the official SDK's interface shapes. This module holds no protocol
  state of its own, so a change here is usually a change in how an SDK type is
  projected.
- Adding a new MCP primitive: expose the SDK type or add a small function; do
  not grow a package structure ahead of the protocol.
