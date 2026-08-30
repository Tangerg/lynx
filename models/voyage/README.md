# voyage

Package voyage implements Scope's embedding and reranking protocols through
Voyage AI's native API. Embedding exposes retrieval task and output-shape
controls at the provider boundary; reranking exposes Voyage truncation through
a typed extension while Core owns the result contract. See
https://docs.voyageai.com/ for the provider reference.

## Install

```bash
go get github.com/Tangerg/scope/models/voyage
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewEmbeddingModel`
- `NewRerankModel`

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
