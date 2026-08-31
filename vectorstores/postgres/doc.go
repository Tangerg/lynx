// Package postgres is the module overview for the PostgreSQL-family vector
// stores. It declares no API of its own; each backend lives in a subpackage.
//
// This one module carries two backends because they share one indivisible
// stack — the pgwire protocol and pgvector — and therefore one dependency
// island and one release cadence:
//
//   - pgvector: PostgreSQL with the pgvector extension.
//   - cockroachdb: CockroachDB over the same wire.
//
// Both implement the capabilities they genuinely have from core/vectorstore and
// compile the shared filter Predicate into their own SQL dialect. Their query
// engine is shared privately through internal/pgstore, so neither backend
// exposes the other's types.
//
// Schema initialization is an explicit switch: on, it creates tables and
// indexes; off, it assumes the schema is provisioned. A store never silently
// alters a backend schema. Every configurable identifier — schema, table,
// index, metadata column — passes SQL identifier validation at construction,
// because those names reach the query text directly and that is the injection
// trust boundary.
package postgres
