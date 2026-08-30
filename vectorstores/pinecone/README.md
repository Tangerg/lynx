# pinecone

Package pinecone exposes Pinecone through the Core vector-store capability
interfaces. Documents are stored as vectors in a Pinecone index (`{id, values,
metadata}`); retrieval runs the index's similarity query. Requirements: a
Pinecone account and an existing index (created via the Pinecone console or
control-plane API — Pinecone does not allow lazy index creation from the data
plane). The store uses the official pinecone-io/go-pinecone v4 client. Vector
similarity. Pinecone configures cosine / dotproduct / euclidean at index-
creation time; the store reads but does not override. Filter visitor produces
Pinecone's metadata-filter syntax — `{"author": {"$eq": "Alice"}}`, `{"$and":
[...]}`, `{"$in": [...]}`. The result feeds the `Filter` field of the query
request. Pinecone has no native LIKE / regex; the visitor rejects filter.OpLike
expressions explicitly. Document text. Pinecone itself stores only id + vector
+ flat metadata — there is no first-class text body. The store always stashes
the original document text under a reserved metadata key; retrieval reverses
the mapping back into document.Document.Text. See https://docs.pinecone.io/ for
the full API surface.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/pinecone
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
