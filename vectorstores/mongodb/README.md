# mongodb

Package mongodb exposes MongoDB Atlas Vector Search through the Core vector-
store capability interfaces. Documents are stored as ordinary BSON documents
(`{_id, content, metadata, embedding}`); retrieval runs the `$vectorSearch`
aggregation stage. Requirements: MongoDB Atlas (vector search isn't available
on self-hosted Community / Enterprise — it's an Atlas-only feature). The store
uses the v2 official driver (go.mongodb.org/mongo-driver/v2). Vector similarity
functions: SimilarityCosine / SimilarityEuclidean / SimilarityDotProduct. The
chosen value is recorded in the Atlas Vector Search index definition. Indexes.
Atlas Vector Search indexes are NOT regular MongoDB indexes; they're managed
via the Search Indexes API and live on dedicated Atlas search nodes. The store
creates one automatically under StoreConfig.InitializeSchema = true, including
any metadata fields enumerated in StoreConfig.MetadataFieldsToFilter as typed
`filter` paths. Filter visitor produces MongoDB query-document syntax —
`{"metadata.author": {"$eq": "Alice"}}`, `{"$and": [...]}`, `{"$nor": [...]}`
for NOT, `{"$regex": ..., "$options": "i"}` for LIKE. The result feeds the
`filter` field of `$vectorSearch`. Search pipeline: {$vectorSearch: {...}},
{$addFields: {score: {$meta: "vectorSearchScore"}}}, {$match: {score: {$gte:
minScore}}} See https://www.mongodb.com/docs/atlas/atlas-vector-search/.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/mongodb
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
