# revai

Package revai wraps Rev AI's speech-to-text API. NewAudioTranscriptionModel
orchestrates Rev AI's async /v1/jobs flow (submit → poll → fetch). Rev AI's
strength is enterprise- grade accuracy on English / multilingual content plus
rich metadata (speaker channels, custom vocabularies, profanity filter,
language ID). Provider extras (speaker_channels_count, custom_vocabulary_id,
language, remove_disfluencies, transcriber selection) ride through extension-
threaded JobOptions fields. See https://docs.rev.ai/ for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/revai
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
