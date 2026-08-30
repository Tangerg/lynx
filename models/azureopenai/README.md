# azureopenai

Package azureopenai adapts Azure OpenAI's current OpenAI-compatible v1 API to
the Core model interfaces. BaseURL is the complete Azure OpenAI v1 base URL,
for example "https://RESOURCE.openai.azure.com/openai/v1/". Model names are
Azure deployment names. Authentication uses the API key through the standard
OpenAI client authentication path accepted by Azure's v1 endpoint. Only the
current v1 endpoint shape is modeled; no dated api-version is required.

## Install

```bash
go get github.com/Tangerg/scope/models/azureopenai
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAudioTTSModel`
- `NewAudioTranscriptionModel`
- `NewChat`
- `NewEmbeddingModel`
- `NewImageModel`

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
