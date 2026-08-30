# fireworks

Package fireworks wraps Fireworks AI's OpenAI-compatible API. Fireworks hosts
open-weight models on its FireAttention serving stack and ships latency-
optimized custom variants of popular models. Fireworks-specific knobs reachable
through the namespaced OpenAI request extension: -
"context_length_exceeded_behavior" controls truncation policy. -
"prompt_cache_max_len" enables Fireworks' prompt-cache layer. - The
/chat/completions endpoint accepts "response_format" with "type":"grammar" to
constrain output via GBNF (alongside the standard "json_schema"). See
https://docs.fireworks.ai/ for the full API reference.

## Install

```bash
go get github.com/Tangerg/scope/models/fireworks
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
