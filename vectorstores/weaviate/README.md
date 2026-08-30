# weaviate

Package weaviate exposes Weaviate through the Core vector-store capability
interfaces. Documents are stored as objects in a Weaviate class (`{id, vector,
properties}`); retrieval runs Weaviate's `nearVector` (or `nearText` when the
class is configured with a vectorizer) query. Requirements: a reachable
Weaviate v5 server (self-hosted or Weaviate Cloud Services). The store uses the
official weaviate-go-client/v5. Vector similarity functions: cosine / dot /
l2-squared / hamming / manhattan. The chosen value is bound to the class's
vector index config at creation time. Schema. Weaviate is strongly typed —
properties participating in filters must be declared at class-creation time.
StoreConfig enumerates these properties so the store can issue a CREATE CLASS
when needed. Filter visitor produces Weaviate's `where` filter operator tree —
`{"operator": "Equal", "path": ["author"], "valueText": "..."}`, `{"operator":
"And", "operands": [...]}`, `{"operator": "GreaterThan", "valueNumber": 100}`.
The result feeds the `WithWhere` builder on the GraphQL Get call. See
https://weaviate.io/developers/weaviate for the full API surface.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/weaviate
```

## Constructors

Every constructor validates its config and returns a value implementing
the capability interfaces in `core/vectorstore`:

- `NewStore`

## Testing

This module integrates a third-party service, so its tests cover what runs
without live credentials: config validation, request and response mapping, and
error classification. The shared conformance contract is
`core/vectorstore/storetest` — this module runs it rather than copying it.

An integration probe skips unless its credential environment variable is set,
so `go test ./...` is always runnable offline.

## Boundaries

This is an independent leaf module: it carries only its own SDK dependency and
never imports a sibling provider. The shared contract every module in this
family obeys is in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
