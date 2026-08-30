# lmnt

Package lmnt wraps LMNT's TTS API. NewAudioTTSModel targets LMNT's
/v1/ai/speech endpoint. LMNT is optimized for ultra-low-latency synthesis
(~300ms first-byte) on the Blizzard and Aurora voice families, with
conversational pacing well suited to voice agents. Provider-specific knobs
(speed, format, sample_rate, conversational mode, language, seed) ride through
extension-threaded SpeechRequest fields. See https://docs.lmnt.com/ for the
full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/lmnt
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
