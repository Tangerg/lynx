# elevenlabs

Package elevenlabs wraps ElevenLabs' voice APIs. Two modalities are exposed: -
/v1/text-to-speech/{voice_id} via NewAudioTTSModel — synthesizes speech from
text. ElevenLabs is voice-first: every call needs a voice id (the cloned or pro
voice) which is supplied through tts.Options.Voice; - /v1/speech-to-text via
NewAudioTranscriptionModel — transcribes audio with speaker diarization,
language id, and timestamps; uses the Scribe v2 model family. ElevenLabs' voice
cloning / library / projects surfaces aren't modeled here — they don't fit
core/model's tts/transcription interfaces. Use the REST API directly for those.
See https://elevenlabs.io/docs/api-reference for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/elevenlabs
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
