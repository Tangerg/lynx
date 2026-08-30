# xai

Package xai wraps xAI's (Grok) OpenAI-compatible API. NewOpenAIChat returns
xAI's provider-local OpenAIChat, backed by the shared OpenAI Chat Completions
protocol. Current Grok models support text, image input, structured outputs,
reasoning effort, and custom function calling through the Chat Completions
surface. xAI's server-side Web Search, X Search, code execution, and
collections tools belong to its Responses API surface and are not represented
as Core custom functions by this adapter. See https://docs.x.ai/ for the full
API reference.

## Install

```bash
go get github.com/Tangerg/scope/models/xai
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
