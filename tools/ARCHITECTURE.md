# tools architecture

> Ready-to-assemble shell, filesystem, HTTP, web-fetch, web-search, and skill
> tools. The single-tool protocol, typed functions, and the instance registry
> belong to `core/tool`; the schema contract belongs to `core/jsonschema` alone.

Repository-wide rules live in [`../CLAUDE.md`](../CLAUDE.md). The tool inventory
and dependency list follow the code; this document states the boundaries only.

---

## 1. Position

- **This module owns concrete tools; Core owns the tool model.**
  `core/tool.Registry` manages the instance set; `tools/*` only implements
  executable capability and never grows a second collection abstraction.
- **Each capability owns its implementation.** `fs`, `shell`, and the rest stay
  separate packages behind a two-tier SPI: the **Tool tier** faces the model
  (JSON in and out, schema, interaction) and the **Backend Port tier** does the
  real work (local, remote, or sandboxed backends are interchangeable). A
  consumer depends on the smallest single-operation port and must never require
  a backend to implement unrelated capability. Demonstration tools live in
  `examples`.
- **External dependencies island by direction and lifecycle.** The neutral
  `Searcher` and `Fetcher` SPIs under `web`, every provider, `httpreq`, and the
  `skills` adapter share this one module because they are all assemblable tools
  with the same dependency direction and release cadence. A package isolates
  capability; a module is not re-cut for the same release unit, and a mature
  third-party library is not rejected on a dependency "budget".

## 2. Mental model

- **The two-tier SPI is the whole design.** The Tool tier does JSON ↔ Go,
  schema validation, and model interaction. *All* domain logic — line numbers,
  binary detection, write locks, the non-expandable path authority — lives in
  the Backend Port tier, so a remote backend can optimize independently instead
  of round-tripping an entire file.
- **Manual registration, no global registry.** A caller registers tools into its
  own toolset; multiple agents and processes each manage their own.
- **One executable Tool identity.** `core/tool.Registry` manages ordinary
  `tool.Tool` values, and the Agent Framework's `interaction.Dispatcher` freezes
  that same set. No second tool type, mutable registry, or bridge.
- **One schema owner.** Prefer deriving the schema from the input struct with
  `core/tool.NewFunc`. A hand-written tool that genuinely cannot fit a typed
  function uses `core/jsonschema` directly, never a forwarding API in this
  module.
- **Typed helpers carry no runtime policy.** `tool.NewFunc` does not handle
  concurrency, retries, human-in-the-loop, direct return, or tool-loop
  termination. Those belong to `agent/interaction` and a Host adapter.
- **Nil-safety is deliberately asymmetric.** A capability with a local
  implementation (`shell`, `fs`) treats `New(nil)` as "use the local backend"
  and works out of the box. A capability that must be configured (`web`,
  `httpreq`) returns an error from `New(nil)`, because there is no safe local
  fallback.
- **Oversized output truncates, it does not fail.** The result carries a
  truncated marker so the model can decide what to do next.
- **Bulk queries belong to the Backend Port.** Glob and grep each have a minimal
  port so a remote backend answers in one RPC instead of many list-then-read
  round trips.
- **Providers share one response shape.** Every web provider returns the same
  `SearchResponse` and `FetchResponse`, so the model never adapts to a vendor
  API.

## 3. Negative invariants

- Never add a global tool registry. Explicit registration is the point.
- Never duplicate the Tool, Registry, or schema primitives here. Tool and
  Registry belong to `core/tool`; schema belongs to `core/jsonschema`.
- Never split a web provider into its own module while it shares this module's
  dependency direction and lifecycle. `web/<provider>` is an implementation
  package, not a release unit. Re-evaluate only when a genuinely heavy SDK or a
  different lifecycle appears.
- Never put domain logic in the Tool tier. It belongs to the Backend Port; the
  Tool tier is JSON ↔ Go plus schema.
- Never add a root restriction to `shell`. The caller is trusted; jailing
  belongs to an outer layer (process context, container).
- Never give `httpreq` a default allowlist. It must be configured explicitly —
  "works even if you forget" is an SSRF hole.
- Never raise an error instead of truncating on an output limit.

## 4. Read before changing

- Changing `chat.ToolDefinition` touches every tool, the registry, and every
  provider request mapping — it is a current `core/chat` protocol value.
- Adding a tool: new subpackage, input struct, tool, factory. The schema is
  derived.
- Adding a backend (remote sandbox, container): implement only the
  single-operation ports it actually provides and inject it at the call site.
  Providing a read must never force implementing write or search.
- Adding a web provider: implement `Searcher`, `Fetcher`, or both under
  `web/<provider>` without touching the Tool tier. One package per provider,
  sharing this module.
