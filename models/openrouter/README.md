# openrouter

Package openrouter wraps OpenRouter — a unified gateway that routes to 100+
LLMs across 50+ providers through a single OpenAI-compatible API. Model ids use
a "provider/model-name" format. ModelAuto delegates model choice to
OpenRouter's task-aware router. Suffixes like ":free", ":nitro", and ":floor"
select cost or latency variants. OpenRouter-specific features the openai facade
plumbs through transparently: - models array for automatic fallback across
alternatives (encode an OpenAI ChatCompletionNewParams value in the namespaced
request extension); - provider preference routing (provider field in extra
body); - transforms (middle-out compression). This facade adds typed knobs for
the two app-attribution headers OpenRouter asks integrations to set (HTTP-
Referer and X-Title) so the calling app shows up on the OpenRouter leaderboard.
See https://openrouter.ai/docs for the full docs.

## Install

```bash
go get github.com/Tangerg/scope/models/openrouter
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
