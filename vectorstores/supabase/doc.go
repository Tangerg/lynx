// Package supabase wraps the [pgvector] store with Supabase-friendly
// defaults. A Supabase database IS Postgres + pgvector — the
// platform installs the extension by default — so this package
// reuses the entire pgvector surface and exists primarily for
// discoverability under the Supabase brand.
//
// Authentication: build a *pgxpool.Pool against the connection
// string from "Project Settings → Database → Connection string →
// URI". Use the Transaction or Session pooler URI in serverless
// environments (the direct connection has connection-count limits).
//
// Row Level Security: lynx makes no assumptions about RLS. If your
// Supabase project enforces it, run as a role that bypasses RLS or
// own the table from a service role.
//
// See https://supabase.com/docs/guides/ai/vector-embeddings.
package supabase
