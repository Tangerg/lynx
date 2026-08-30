# deepseek

Package deepseek wraps DeepSeek's OpenAI-compatible API. DeepSeek derives from
the OpenAI Chat Completions protocol while retaining its provider-specific
reasoning semantics behind OpenAIChat. Provider-specific behavior handled
transparently: - reasoning_content is decoded into a [chat.PartReasoning]; -
ordinary prior assistant reasoning is omitted on later turns; - reasoning
associated with tool calls is replayed as reasoning_content. Provider-specific
request controls use the typed RequestOptions extension; the OpenAI SDK request
shape is intentionally not exposed. Prefix completion remains a separate beta
protocol and is not accepted by OpenAIChat. See https://api-docs.deepseek.com/
for the full API reference.

## Install

```bash
go get github.com/Tangerg/scope/models/deepseek
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
