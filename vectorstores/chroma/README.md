# chroma

Package chroma exposes Chroma through the Core vector-store capability
interfaces. Documents are stored as records inside a Chroma collection (`{id,
document, embedding, metadata}`); retrieval runs the collection's nearest-
neighbor query. Requirements: a reachable Chroma server (self-hosted or Chroma
Cloud). The store uses the official Go client over HTTP. Vector similarity
functions: cosine / L2 / inner-product. The chosen value is recorded in the
collection metadata at creation time and cannot be changed without rebuilding
the collection. Filter visitor produces Chroma's flat where-clause syntax —
`{"$and": [...]}`, `{"author": {"$eq": "Alice"}}`, `{"$contains": "..."}` for
LIKE. The result feeds the `where` field on the query call. Metadata fields are
addressed at the top level (no `metadata.` prefix); Chroma stores metadata
flat. See https://docs.trychroma.com/ for the full API surface.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/chroma
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
