// Package cockroachdb wraps the [pgvector] store with CockroachDB-
// friendly defaults. CockroachDB v25+ ships native VECTOR support
// over the PostgreSQL wire protocol — column type, distance
// operators (<->, <=>, <#>), and HNSW index syntax are all
// pgvector-compatible. The two differences worth wrapping:
//
//  1. Cockroach has no `CREATE EXTENSION` step (vector support is
//     built in), so the wrapper sets [pgvector.StoreConfig.SkipExtensionCreate]
//     automatically.
//  2. Cockroach connections are still pgx-driven — callers point a
//     standard *pgxpool.Pool at their Cockroach cluster's
//     `postgresql://` URL.
//
// Use [NewStore] when you want a thin, well-documented CockroachDB
// entry point. Power users can keep using [pgvector.NewStore]
// directly and set SkipExtensionCreate themselves.
package cockroachdb

import (
	"github.com/Tangerg/lynx/vectorstores/pgvector"
)

const Provider = "CockroachDB"

// StoreConfig mirrors [pgvector.StoreConfig] minus the fields that
// don't apply to CockroachDB (extension creation). All other fields
// — schema name, table name, distance metric, index type, etc. —
// behave exactly as documented on the pgvector counterpart.
type StoreConfig = pgvector.StoreConfig

// Store is a CockroachDB-backed the vectorstore capability interfaces implementation,
// inheriting every method from [pgvector.Store].
type Store = pgvector.Store

func NewStore(cfg StoreConfig) (*Store, error) {
	cfg.SkipExtensionCreate = true
	return pgvector.NewStore(cfg)
}
