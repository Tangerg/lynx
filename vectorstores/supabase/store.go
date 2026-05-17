// Package supabase wraps the [pgvector] store with Supabase-friendly
// defaults. Supabase databases are vanilla Postgres + the pgvector
// extension installed by the Supabase platform, so this package
// reuses the entire [pgvector] surface and exists primarily for
// discoverability.
//
// Authentication: build a *pgxpool.Pool against the connection
// string Supabase provides under "Project Settings → Database →
// Connection string → URI" (use the "Transaction" or "Session"
// pooler URI in serverless environments). The wrapper makes no
// assumptions about RLS / service-role keys — those are governed at
// the connection-string level.
package supabase

import (
	"github.com/Tangerg/lynx/vectorstores/pgvector"
)

const Provider = "Supabase"

// StoreConfig is a type alias of [pgvector.StoreConfig]; every field
// behaves identically. Supabase ships the pgvector extension
// pre-installed but typically requires a one-time `CREATE EXTENSION`
// run per database, which the default [pgvector.StoreConfig]
// already handles via InitializeSchema.
type StoreConfig = pgvector.StoreConfig

// Store is a Supabase-backed [vectorstore.Store] implementation. It's
// identical to [pgvector.Store] at runtime.
type Store = pgvector.Store


func NewStore(cfg *StoreConfig) (*Store, error) {
	return pgvector.NewStore(cfg)
}
