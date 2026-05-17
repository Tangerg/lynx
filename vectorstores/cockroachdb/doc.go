// Package cockroachdb wraps the [pgvector] store with
// CockroachDB-friendly defaults. CockroachDB v25+ ships native
// VECTOR support over the PostgreSQL wire protocol — column type,
// distance operators (<->, <=>, <#>), and HNSW index syntax are
// pgvector-compatible.
//
// What's different from the upstream pgvector wrapper:
//
//   - Cockroach has no extension system; the wrapper sets
//     [pgvector.StoreConfig.SkipExtensionCreate] automatically so
//     the initializer doesn't issue `CREATE EXTENSION vector`.
//   - Everything else (schema layout, distance operators, HNSW
//     index, jsonb metadata, filter visitor) is identical to
//     [pgvector].
//
// Requirements: CockroachDB v25 or later. Connect a regular pgx
// pool to the `postgresql://` URL from your cluster.
//
// See https://www.cockroachlabs.com/docs/stable/vector.html for the
// Cockroach-side reference.
package cockroachdb
