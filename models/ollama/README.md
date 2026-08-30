# ollama

Package ollama wraps Ollama's two chat surfaces. Ollama serves the same models
at two different wire formats: - Native API at /api/chat — accessed via
NewChat. Gives access to Ollama-specific features (keep_alive, format=json,
thinking on supported models, raw "options" dict for fine-grained sampling
control). - OpenAI-compatible API at /v1/chat/completions — accessed via
NewOpenAIChat. Works with the same Core chat protocol and benefits from the
openai provider's response_format / tool_calling / reasoning_content plumbing.
Pick native when the daemon-specific knobs matter, OpenAI-compat when
integrating with code already written against the openai API. Embedding has the
same dual surface; scope ships the native flavor as NewEmbeddingModel. The
OpenAI-compatible /v1/embeddings path works through openai.NewEmbeddingModel
with [option.WithBaseURL] pointed at "http://host:11434/v1". The native
adapters own a narrow private HTTP wire for /api/chat and /api/embed. They do
not import the Ollama daemon repository; server implementation packages are not
a client abstraction.

## Install

```bash
go get github.com/Tangerg/scope/models/ollama
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewChat`
- `NewEmbeddingModel`
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
