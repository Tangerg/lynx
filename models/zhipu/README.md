# zhipu

Package zhipu wraps Zhipu AI's chat APIs (GLM family). BigModel serves the GLM
chat surface in two compatibility flavors: - OpenAI-compatible at /api/paas/v4
— use NewOpenAIChat; - Anthropic-compatible at /api/anthropic — use
NewAnthropicChat, available for GLM-4.5 and GLM-4.6. Swap base URL and keep
their existing integration. Embedding (embedding-3 / embedding-2) only has the
OpenAI flavor and goes through NewEmbeddingModel. Zhipu-specific surfaces
(CogView image generation, CogVideoX video) sit on separate endpoints and
aren't exposed by this package. See https://docs.bigmodel.cn/ for the API
reference.

## Install

```bash
go get github.com/Tangerg/scope/models/zhipu
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAnthropicChat`
- `NewEmbeddingModel`
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
