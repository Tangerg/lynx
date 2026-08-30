# alibaba

Package alibaba wraps Alibaba Cloud's DashScope model platform, which hosts the
Qwen / Tongyi family and several other Alibaba models. DashScope exposes two
surfaces: - the native /api/v1/services/aigc/text-generation/generation
endpoint with a DashScope-specific JSON shape (not used here); - the
/compatible-mode/v1 path which speaks the OpenAI chat-completions / embeddings
spec. This package uses the compatible-mode endpoint to route through the
openai provider facade. DashScope-specific knobs (enable_thinking,
enable_search, web search citations, etc.) use the namespaced OpenAI request
extension. See https://help.aliyun.com/zh/model-studio/ for the docs.

## Install

```bash
go get github.com/Tangerg/scope/models/alibaba
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

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
