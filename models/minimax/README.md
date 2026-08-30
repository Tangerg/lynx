# minimax

Package minimax wraps MiniMax's chat APIs. MiniMax operates two billing zones
(international USD / domestic RMB) and exposes the chat surface in two
compatibility flavors: - OpenAI-compatible at /v1 — use NewOpenAIChat; -
Anthropic-compatible at /anthropic — use NewAnthropicChat, which routes through
the anthropic provider so the Anthropic SDK's tool-calling, extended thinking,
and reasoning-signature handling all work as-is. MiniMax-specific surfaces
(Text-to-Speech, Voice Clone, Image generation, Video generation) are separate
endpoints with custom wire formats not exposed by this package. See
https://platform.minimaxi.com/document for the full API.

## Install

```bash
go get github.com/Tangerg/scope/models/minimax
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAnthropicChat`
- `NewOpenAIChat`

## Testing

This module integrates a third-party service, so its tests cover what runs
without live credentials: config validation, request and response mapping, and
error classification. The shared conformance contract is `core/modeltest` for
behavior and `dev/providerconformance` for construction and API consistency —
this module runs it rather than copying it.

An integration probe skips unless its credential environment variable is set,
so `go test ./...` is always runnable offline.

## Boundaries

This is an independent leaf module: it carries only its own SDK dependency and
never imports a sibling provider. The shared contract every module in this
family obeys is in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
