// Package pgvector wraps the pgvector PostgreSQL extension as a
// the vectorstore capability interfaces. Documents live in a regular Postgres table
// with a typed `vector(N)` column; metadata is stored in a `jsonb`
// column and reached through `metadata->>'key'` extractors.
//
// Requirements: PostgreSQL 13+ with the `vector` extension installed
// (the store runs `CREATE EXTENSION IF NOT EXISTS vector` under
// [StoreConfig.InitializeSchema] = true).
//
// Distance metrics — three of pgvector's six operators are exposed:
//
//   - [DistanceCosine] (`<=>`) — cosine distance, `vector_cosine_ops`
//   - [DistanceL2]     (`<->`) — Euclidean distance, `vector_l2_ops`
//   - [DistanceIP]     (`<#>`) — negative inner product, `vector_ip_ops`
//
// Index types: HNSW (default, best query perf) / IVFFlat (faster
// builds) / none (exact sequential-scan). Vector binding uses
// pgvector-go's typed [pgvec.Vector]; the connection is a standard
// pgx pool.
//
// Filter visitor builds parameterized SQL — values flow through `$N`
// placeholders so untrusted input is safe. JSON path access reads
// numbers via `::numeric` cast and booleans via `::boolean`.
//
// See https://github.com/pgvector/pgvector for the extension docs.
package pgvector
