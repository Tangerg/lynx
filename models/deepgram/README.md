# deepgram

Package deepgram wraps Deepgram's speech APIs. Two modalities are exposed: -
/v1/listen via NewAudioTranscriptionModel — high-throughput real-time / batch
transcription on the Nova family. Provider- specific knobs (smart_format,
diarize, utterances, paragraphs, redact, keywords) ride through extension-
threaded params; - /v1/speak via NewAudioTTSModel — synthesis on the Aura voice
family. Deepgram's live streaming (WebSocket) and analyze surfaces aren't
modeled here — the WebSocket flow doesn't fit core/model's request/ response
shape. See https://developers.deepgram.com/docs for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/deepgram
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAudioTTSModel`
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
