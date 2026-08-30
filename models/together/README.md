# together

Package together wraps Together AI's OpenAI-compatible API. Together hosts
hundreds of open-weight models (Llama, DeepSeek, Qwen, Mistral, etc.) with
serverless and dedicated endpoints. Together-specific knobs reachable through
the namespaced OpenAI request extension: - "echo" / "min_p" /
"repetition_penalty" / "top_k" are accepted on top of the standard openai
surface. - The "safety_model" field enables Llama Guard prefilter / postfilter
by naming a guard model. See https://docs.together.ai/ for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/together
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
