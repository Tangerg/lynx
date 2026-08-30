# voyage

Package voyage wraps Voyage AI's embedding API. Voyage publishes retrieval-
tuned text and multimodal embedding models that consistently lead public
retrieval benchmarks; the current voyage-4-large / voyage-4 / voyage-4-lite
models support matryoshka-style output truncation via the output_dimension
parameter. Voyage's /embeddings shape is bespoke (input_type, truncation,
quantization knobs) and doesn't speak the OpenAI dialect — this package
implements embedding.Model directly against the native API. See
https://docs.voyageai.com/ for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/voyage
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
