// Package mariadb wraps MariaDB's native VECTOR column type as a
// the vectorstore capability interfaces. Documents live in a regular MariaDB table
// (id / content / metadata JSON / embedding VECTOR) reached through
// `database/sql` + the go-sql-driver/mysql driver.
//
// Requirements: MariaDB 11.6+ (vector support landed in 11.6 GA;
// the VECTOR INDEX HNSW backing only became stable in 11.7).
//
// Distance metrics: [DistanceCosine] (uses `vec_distance_cosine`) /
// [DistanceEuclidean] (uses `vec_distance_euclidean`). Both are
// honored by the HNSW index when present.
//
// Vector binding. MariaDB accepts vectors through the `VEC_FromText`
// function — the store renders `[v1,v2,...]` as a literal and lets
// MariaDB parse it. Typed binary binding isn't exposed by the Go
// driver yet, but the textual form is fully supported.
//
// Filter visitor reaches into the JSON metadata column with
// `JSON_VALUE(metadata, '$.k')`, wrapping numeric comparisons in
// `CAST(... AS DOUBLE)` so range queries don't fall back to
// lexicographic ordering.
//
// See https://mariadb.com/kb/en/vector-overview/ for the official
// reference.
package mariadb
