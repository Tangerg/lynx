# mariadb

Package mariadb exposes MariaDB's native VECTOR column type through the Core
vector-store capability interfaces. Documents live in a regular MariaDB table
(id / content / metadata JSON / embedding VECTOR) reached through
`database/sql` + the go-sql-driver/mysql driver. Requirements: MariaDB 11.6+
(vector support landed in 11.6 GA; the VECTOR INDEX HNSW backing only became
stable in 11.7). Distance metrics: DistanceCosine (uses `vec_distance_cosine`)
/ DistanceEuclidean (uses `vec_distance_euclidean`). Both are honored by the
HNSW index when present. Vector binding. MariaDB accepts vectors through the
`VEC_FromText` function — the store renders `[v1,v2,...]` as a literal and lets
MariaDB parse it. Typed binary binding isn't exposed by the Go driver yet, but
the textual form is fully supported. Filter visitor reaches into the JSON
metadata column with `JSON_VALUE(metadata, '$.k')`, wrapping numeric
comparisons in `CAST(... AS DOUBLE)` so range queries don't fall back to
lexicographic ordering. See https://mariadb.com/kb/en/vector-overview/ for the
official reference.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/mariadb
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
