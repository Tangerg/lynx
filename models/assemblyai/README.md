# assemblyai

Package assemblyai wraps AssemblyAI's speech-to-text API.
NewAudioTranscriptionModel orchestrates the upload → submit → poll → fetch flow
against AssemblyAI's async /v2/transcript endpoints.
transcription.Options.Model selects the primary official model; use
"universal-3-5-pro" for the current frontier model and add "universal-2" to
[TranscriptRequest.SpeechModels] as a fallback. Provider extras (speaker
diarization, auto chapters, sentiment analysis, entity detection, PII
redaction, content safety, language detection) ride through extension-threaded
TranscriptRequest fields. See https://www.assemblyai.com/docs for the full
reference.

## Install

```bash
go get github.com/Tangerg/scope/models/assemblyai
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
