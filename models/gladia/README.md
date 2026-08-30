# gladia

Package gladia wraps Gladia's speech-to-text API. NewAudioTranscriptionModel
orchestrates Gladia's async pipeline (upload → /v2/transcription submit → poll
→ fetch). Gladia's supports the official Solaria-3 and Solaria-1 models.
Solaria-1 provides broad multilingual coverage and code switching; Solaria-3
targets one configured European language. Speaker diarization and add-ons like
summarization, translation, named-entity recognition, and audio intelligence —
all reachable via extension-threaded TranscriptionRequest fields. See
https://docs.gladia.io/ for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/gladia
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAudioTranscriptionModel`

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
