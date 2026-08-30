# groq

Package groq wraps Groq's OpenAI-compatible API. Groq runs open- weight models
(Llama, Gemma, DeepSeek, Kimi) on its in-house LPUs at extremely high
throughput. Groq-specific knobs reachable through the namespaced OpenAI request
extension: - service_tier ("on_demand" / "flex" / "auto") trades cost for
latency. See https://console.groq.com/docs/flex-processing. - reasoning_format
("parsed" / "raw" / "hidden") controls how reasoning-model output is surfaced.
See https://console.groq.com/docs/ for the full API reference.

## Install

```bash
go get github.com/Tangerg/scope/models/groq
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewOpenAIChat`

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
