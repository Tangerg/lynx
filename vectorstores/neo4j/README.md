# neo4j

Package neo4j exposes the official neo4j-go-driver v5 through the Core vector-
store capability interfaces. Documents become nodes labeled `:Document` (or
whatever StoreConfig.Label picks) — metadata keys are stored as flat properties
named `metadata.<key>`, the embedding rides on the configured property, and the
id has a uniqueness constraint. Requirements: Neo4j 5.13+ for `CREATE VECTOR
INDEX` and the `db.index.vector.queryNodes` procedure. Earlier 5.x releases
ship the procedure under a different signature; the store hard-codes the 5.13+
shape. Similarity functions: SimilarityCosine / SimilarityEuclidean. Both are
mapped to a [0, 1] similarity score by Neo4j itself. Indexing — the store
creates two things under StoreConfig.InitializeSchema = true: - a uniqueness
constraint on the id property - a `VECTOR INDEX` carrying dimensions +
similarity function Search calls `CALL db.index.vector.queryNodes($index, $k,
$vec) YIELD node, score WHERE score >= $threshold AND <filter>`. The filter
visitor produces a Cypher predicate plus a `$pN`-keyed parameter map (Cypher
uses named parameters). LIKE maps onto Cypher's `=~` (regex). Note that NOT in
Cypher must precede an expression — the visitor emits `NOT (<expr>)`. See
https://neo4j.com/docs/cypher-manual/current/indexes-for-vector-search/ for
index syntax and the vector-search reference.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/neo4j
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
