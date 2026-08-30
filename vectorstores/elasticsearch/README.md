# elasticsearch

Package elasticsearch exposes the official go-elasticsearch v8 client through
the Core vector-store capability interfaces. Documents are indexed JSON objects
with a `dense_vector` field for the embedding and a nested `object` field for
metadata. Requirements: Elasticsearch 8.0+ for dense_vector + `knn` top-level
query. The store uses the `knn` query (not `script_score`) for retrieval —
that's GA since 8.4. Similarity functions: SimilarityCosine / SimilarityL2 /
SimilarityDotProduct. The chosen value is recorded in the dense_vector mapping
at index creation time and cannot be changed without rebuilding. Search shape:
POST <index>/_search { "size": K, "knn": { "field": "embedding",
"query_vector": [...], "k": K, "num_candidates": ceil(K *
NumCandidatesMultiplier), "filter": {"query_string": {"query": "<lucene>"}} } }
Filter visitor produces Lucene query-string syntax — metadata fields are
addressed under `metadata.<key>` paths; LIKE wildcards (% / _) map to Lucene
wildcards (* / ?). Delete uses _delete_by_query with the same Lucene filter.
See https://www.elastic.co/docs/reference for the full API.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/elasticsearch
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
