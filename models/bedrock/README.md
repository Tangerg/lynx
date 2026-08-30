# bedrock

Package bedrock wraps AWS Bedrock Runtime. Bedrock is a model-aggregation
gateway — a single endpoint that fronts foundation models from Anthropic, Meta,
Mistral, Amazon Titan / Nova, Cohere, AI21, Stability and others. NewChat uses
the unified Converse / ConverseStream API which speaks a provider-agnostic
message shape; NewEmbeddingModel targets the native InvokeModel contracts for
Titan Text Embeddings V1/V2 and Cohere Embed V3/V4. Provider-only embedding
controls use EmbeddingRequestOptions under EmbeddingRequestExtensionKey. Model
selection is via the upstream model id (e.g.
"anthropic.claude-3-5-sonnet-20241022-v2:0", "amazon.titan-embed-text-v2:0",
"meta.llama3-1-70b-instruct-v1:0", "us.anthropic.claude-
sonnet-4-20250514-v1:0"). Bedrock supports regional and cross-region inference-
profile IDs. AWS auth is handled by the standard aws-sdk-go-v2 chain (env vars,
shared config, IRSA, instance role); no custom APIKey is required. See
https://docs.aws.amazon.com/bedrock/ for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/bedrock
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewChat`
- `NewEmbeddingModel`
- `NewReasoningPart`
- `NewRedactedReasoningPart`

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
