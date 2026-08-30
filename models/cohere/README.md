# cohere

Package cohere implements Scope's embedding and reranking protocols through
Cohere's v2 API. Embedding callers select the official input type explicitly;
reranking returns input indices and normalized relevance without duplicating
documents. Cohere-specific token and priority controls use the provider-owned
`RerankRequestOptions` extension rather than leaking the SDK request type. See
https://docs.cohere.com/ for the provider reference.

## Install

```bash
go get github.com/Tangerg/scope/models/cohere
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
