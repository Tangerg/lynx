# mistral

Package mistral wraps Mistral AI's API. Mistral exposes: - /chat/completions —
native structured content and thinking chunks, used via NewChat; - /embeddings
— OpenAI-compatible, used via NewEmbeddingModel (returns an
openai.EmbeddingModel); - /moderations — Mistral-native shape that doesn't
match OpenAI's moderation response; NewModerationModel handles it directly
against [API] from this package. Additional Mistral surfaces not exposed here:
- /agents (stateful agent runs); - /fim (code completion endpoint; it is not
modeled by the chat facade). See https://docs.mistral.ai/ for the full API
reference.

## Install

```bash
go get github.com/Tangerg/scope/models/mistral
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewChat`
- `NewEmbeddingModel`
- `NewModerationModel`

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
