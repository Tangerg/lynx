# jina

Package jina wraps Jina AI's native embedding API. It supports the dense float
subset of current v5, v4, v3, CLIP, and code embedding models that maps
losslessly to core/embedding. Jina-specific knobs that don't fit the generic
surface — task type ("retrieval.query" / "retrieval.passage" / "text-matching"
/ "classification" / "separation"), late chunking, embedding_type (float / int8
/ uint8 / binary / ubinary quantization), normalization — are reached through
EmbeddingRequestExtensionKey and [EmbeddingRequest]. Jina's /embeddings dialect
partially overlaps OpenAI's but uses "dimensions" rather than
"output_dimension" and exposes the task-conditioning field; this package
implements embedding.Model directly against the native API. See
https://jina.ai/embeddings/ for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/jina
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewEmbeddingModel`

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
