# anthropic

Package anthropic exposes Anthropic's native Messages adapter, token estimator,
and first-party OpenAI-compatible endpoint. All constructors return Anthropic-
owned types; shared wire implementations remain private to the models module.

## Install

```bash
go get github.com/Tangerg/scope/models/anthropic
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewChat`
- `NewOpenAIChat`
- `NewTextEstimator`

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
