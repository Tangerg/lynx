# openai

Package openai exposes OpenAI's native chat, Responses, embedding, image,
moderation, speech, transcription, and translation adapters. Every constructor
returns an OpenAI-owned model type. Reusable wire behavior remains private to
the models module, so compatible providers do not leak OpenAI concrete types
through their APIs.

## Install

```bash
go get github.com/Tangerg/scope/models/openai
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAudioTTSModel`
- `NewAudioTranscriptionModel`
- `NewAudioTranslationModel`
- `NewChat`
- `NewEmbeddingModel`
- `NewImageModel`
- `NewModerationModel`
- `NewResponsesChat`

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
