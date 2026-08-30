# hume

Package hume wraps Hume AI's TTS API. NewAudioTTSModel targets Hume's /v0/tts
endpoint backed by the Octave voice model — Hume's pitch is emotion-aware
synthesis driven by acting / description prompts in addition to plain text.
Provider-specific knobs (description, voice (named or cloned),
trailing_silence, format, timestamps, and instant_mode) ride through extension-
threaded [TTSRequest] fields. [speech.Options].Model selects the official
Octave protocol version ("1" or "2"). Streaming uses Hume's newline-delimited
/v0/tts/stream/json endpoint directly. Hume's broader expression-measurement
APIs (face / voice / language emotion analysis) aren't exposed — they don't fit
core/model's tts/transcription interfaces. See https://dev.hume.ai/docs for the
full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/hume
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAudioTTSModel`

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
