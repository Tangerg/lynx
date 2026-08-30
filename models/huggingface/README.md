# huggingface

Package huggingface exposes the HuggingFace Inference Router, which is OpenAI-
compatible — chat completions hit /v1/chat/completions with the same
request/response shape. NewOpenAIChat returns the provider-local OpenAIChat,
backed by the shared OpenAI Chat Completions protocol and configured for the
Hugging Face router. Callers receive tool calling and streaming without
depending on OpenAI's concrete adapter type.

## Install

```bash
go get github.com/Tangerg/scope/models/huggingface
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
