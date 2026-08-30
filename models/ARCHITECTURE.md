# models architecture

> One adaptation layer for every LLM, embedding, image, and audio provider. Each
> provider is an independent leaf module implementing the `Model` contracts
> defined by Core.

Repository-wide rules live in [`../AGENTS.md`](../AGENTS.md). The provider
inventory, maturity, and dependency versions follow the code; this document
states the shared contract every `models/<provider>` module obeys. The usage
entry point is [`README.md`](README.md).

---

## 1. Position

- **Converge heterogeneous SDKs onto Core's `Model` contracts.** A consumer sees
  `chat.Model` and its siblings, never the shape of a particular vendor SDK.
- **`models` is only a namespace.** There is no aggregate `models` module. Each
  `models/<provider>` is a leaf module a consumer can select, release, and
  upgrade on its own.
- **Adding a provider means copying the existing structure and swapping the SDK
  calls.** The Scope protocol is never changed for one vendor.

## 2. Mental model

- **One self-contained package per provider, with the same three parts:** a
  `Config` (validation plus factory), a `Model` implementing the Core contract,
  and the endpoint and dialect wiring. A native adapter owns its own `Model`
  type. An endpoint that explicitly speaks the same OpenAI or Anthropic wire
  promotes the shared `Model` through a type alias — never an empty struct that
  only forwards `Call` and `Stream`.
- **The wire protocol is a lower layer.** The OpenAI and Anthropic wires reused
  across providers live in the `models/protocol/openai` and
  `models/protocol/anthropic` modules. A provider may depend downward on a
  protocol; a protocol never depends on a provider; and providers never import
  each other. A public `Config`, DTO, or method signature must not leak a wire
  or SDK type — but, exactly as the MCP Go SDK promotes its underlying message
  types, an identical shared executable `Model` may be promoted by alias. A
  provider-private `internal/protocol` stays wrapped by the outer `Model`.
- **No common base class.** Vendor SDK shapes differ more than they resemble
  each other; a forced shared helper is false DRY. Repetition per provider is
  preferred.
- **Adaptation strategies come in tiers** — use them to decide where a new
  provider lands: native on the vendor SDK; delegating to the OpenAI client with
  a different base URL; a provider exposing both OpenAI and Anthropic APIs; a
  managed platform authenticating through IAM rather than an API key; a local
  container.
- **Options resolve in two levels.** The model's default configuration applies
  one request-level override through `Options.Resolve`. Provider-specific
  parameters go through a typed extractor rather than a manual type assertion.
- **Streaming accumulates event by event.** A native provider — or the owner of
  a shared protocol — stitches SSE deltas into chunks, and the layer above
  assembles a complete message, all through `iter.Seq2` rather than channels. A
  compatible provider does not copy an accumulator for the same wire.
- **Capability gaps are filled per provider.** A reasoning signature, which some
  vendors require to continue a stream and others do not have, is carried as
  neutral bytes rather than forced into a uniform shape.
- **Shared contract tests belong to the contract owner.** The behavior suite is
  `core/modeltest` and cross-provider construction and API consistency is
  `dev/providerconformance`. A provider module copies neither, and decodes its
  extension parameters directly with `core/metadata.Map.Decode`. Provider-local
  transport helpers stay in their own module.

## 3. Negative invariants

- Never share a request or response mapper across different wire protocols. The
  shapes differ more than they overlap, so sharing is false DRY. Reuse a
  protocol implementation only for an endpoint that explicitly claims
  compatibility with that official wire.
- Never add a retry layer. The SDKs already retry.
- Never add OAuth or token refresh to a provider. The user supplies a key, and a
  401 is a prompt to re-enter it.
- Never disguise defaults or metadata as a `Model` capability. `core/chat.Model`
  has only `Call`; defaults live in the provider's construction config, and a
  per-request override is an ordinary `chat.Options` value.
- Never duplicate test infrastructure or wrap metadata decoding in another
  layer. There is exactly one contract suite, and a provider decodes its own
  extension types at its boundary.
- Never create a single-field forwarding shell for a shared wire `Model`. A
  compatible provider owns only its config, validation, endpoint, and dialect;
  the `Model` is promoted by alias. Keep a façade only for `internal/protocol`
  or when the provider genuinely holds state or behavior.

## 4. Read before changing

- Changing a Core chat or `Model` interface forces every provider to follow.
  Estimate the adaptation cost first.
- Adding a provider: take the most complete existing one as the reference, copy
  the three parts, and do not change the shape.
- Changing stream accumulation: it is per provider — run that provider's stream
  tests.
