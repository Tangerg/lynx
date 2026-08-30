# milvus

Package milvus exposes Milvus / Zilliz Cloud through the Core vector-store
capability interfaces. Documents are stored as rows in a Milvus collection
(`{id, content, embedding, <metadata columns>}`); retrieval runs Milvus's ANN
search. Requirements: a reachable Milvus 2.x server (self-hosted, Docker, or
Zilliz Cloud managed service). The store uses the official milvus-sdk-go/v2
gRPC client. Vector similarity functions: cosine / L2 / IP. The chosen value is
bound to the collection's index at creation time; switching requires rebuilding
the index. Schema. Milvus is strongly typed — every metadata field that
participates in filters must be declared as a typed column at schema-creation
time. [StoreConfig.MetadataFields] enumerates the columns; anything outside
that set goes into a flexible JSON field that can still be filtered but at a
higher cost. Filter visitor produces Milvus's expression language — `author ==
"Alice" and (year > 2020 or tag in ["a","b"])`. The result feeds the `expr`
parameter of the search call. See https://milvus.io/docs for the full API
surface.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/milvus
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
