// Package cockroachdb provides a native CockroachDB vector-store adapter over
// pgwire. It uses CockroachDB's built-in VECTOR type, pgvector-compatible
// distance operators, and native VECTOR INDEX declaration.
//
// Requirements: CockroachDB v25.4 or later is recommended for generally
// available vector indexing. Schema initialization is explicit through
// StoreConfig.InitializeSchema.
package cockroachdb
