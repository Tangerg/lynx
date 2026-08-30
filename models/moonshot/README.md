# moonshot

Package moonshot wraps Moonshot AI's Kimi chat APIs. Moonshot serves the same
models at two compatibility flavors and two billing regions: - OpenAI-
compatible at /v1 — use NewOpenAIChat; - Anthropic-compatible at /anthropic —
use NewAnthropicChat, supported on Kimi-K2 and newer reasoning models. Allows
Anthropic-SDK callers to swap base URL. Use BaseURL / BaseURLAnthropic for the
domestic Chinese region and BaseURLIntl / BaseURLIntlAnthropic for the
international (api.moonshot.ai) host. See https://platform.kimi.com/docs for
the docs.

## Install

```bash
go get github.com/Tangerg/scope/models/moonshot
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewAnthropicChat`
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
