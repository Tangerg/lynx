# xiaomi

Package xiaomi wraps Xiaomi's MiMo API open platform. MiMo serves the current
V2.5 and V2.5 Pro models at two compatibility flavors on the same host: -
OpenAI-compatible at /v1 — use NewOpenAIChat; - Anthropic-compatible at
/anthropic — use NewAnthropicChat, which routes through the anthropic provider
so the Anthropic SDK's tool-calling, extended thinking, and reasoning-signature
handling all work as-is. Provider-specific thinking is configured with
ChatRequestOptions under RequestExtensionKey. reasoning_content is surfaced as
Core reasoning and replayed on tool-call turns as required by MiMo's protocol.
MiMo-specific surfaces not exposed here (TTS / image / omni I/O) require
provider-specific request shapes that don't map onto the OpenAI chat-
completions wire. Use the platform's dedicated endpoints directly for those.
See https://mimo.mi.com/docs for the full API reference.

## Install

```bash
go get github.com/Tangerg/scope/models/xiaomi
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
