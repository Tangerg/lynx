# opensearch

Package opensearch exposes the official opensearch-go v4 client through the
Core vector-store capability interfaces. Documents are indexed JSON objects
with a `knn_vector` field for the embedding and a nested `object` field for
metadata. Requirements: OpenSearch 2.x+ with the k-NN plugin (built-in on every
recent release). Space types — five distance variants are recognized; coverage
depends on the engine: - SpaceTypeCosine / SpaceTypeL2 / SpaceTypeIP —
supported by all three engines (Lucene / NMSLib / FAISS); - SpaceTypeL1 /
SpaceTypeLInf — NMSLib and FAISS only. Engines: EngineLucene (default, ships
with core), EngineNMSLib, EngineFaiss. The chosen value is baked into the index
mapping at creation time and cannot be changed without rebuilding. Search uses
approximate k-NN: POST <index>/_search { "size": K, "query": {"knn":
{"embedding": { "vector": [...], "k": K, "filter": {"query_string": {"query":
"<lucene>"}} }}} } Filter visitor produces Lucene query-string syntax under the
configured metadata prefix — same dialect as the Elasticsearch store,
intentionally so callers can swap between the two. See
https://docs.opensearch.org/latest/search-plugins/knn/ for the k-NN plugin
reference.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/opensearch
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
