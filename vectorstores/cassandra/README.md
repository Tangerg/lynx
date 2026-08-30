# cassandra

Package cassandra exposes Apache Cassandra 5.0+ vector support through the Core
vector-store capability interfaces. Documents live in a regular CQL table with
a `vector<float, N>` column; metadata keys must be declared as typed columns
(Cassandra has no JSON-path operator), each indexed via a Storage Attached
Index (SAI). Requirements: Apache Cassandra 5.0+ or compatible (DataStax Astra
DB / DataStax Enterprise). Vector + SAI both arrived together in 5.0. The store
uses gocql v1.x. Similarity functions — recorded in the SAI index definition at
creation time: - SimilarityCosine — cosine similarity (default) -
SimilarityDotProduct — inner product - SimilarityEuclidean — Euclidean distance
Vector binding caveat. gocql v1.x has no first-class `vector<float, N>` codec,
so the store inlines vectors as CQL literals (`[v1, v2, ...]`) into the SQL.
Cassandra accepts that form for both INSERT and ORDER BY ANN OF. The other
parameters flow through normal `?` placeholders. Filter constraints. CQL on
regular columns doesn't support `OR` or standalone `NOT`; the visitor rejects
them with a clear error. `IN` is fine and binds as a typed slice. Every
filterable metadata key must exist as a typed column on the table, declared via
MetadataColumn entries with their CQL type (text / int / boolean / double / …).
Filter-based DELETE. Cassandra forbids deleting by a non-PK predicate. The
store works around it by SELECT-ing matching ids first then issuing per-row
DELETEs. See https://cassandra.apache.org/doc/latest/cassandra/vector-search/
for the official reference.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/cassandra
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
