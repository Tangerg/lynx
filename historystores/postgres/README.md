# postgres

Package postgres is a history Store backed by PostgreSQL via pgx. Each
conversation's messages live in a single table; messages are serialized to
JSONB through the shared tagged core/chat wire codec, so ordered parts, tool
results, media, and metadata round-trip with full fidelity. Historical wire
must be migrated before upgrading; this package reads and writes only the
current tagged format. Example: pool, _ := pgxpool.New(ctx, "postgres://...")
store, _ := postgres.NewStore(ctx, postgres.StoreConfig{ Pool:
pool, InitializeSchema: true, // create the table+index on first use }) defer
pool.Close()

## Install

```bash
go get github.com/Tangerg/scope/historystores/postgres
```

## Constructors

Every constructor validates its config and returns a value implementing
the store capabilities in `core/history`:

- `NewStore`

## Testing

This module integrates a third-party service, so its tests cover what runs
without live credentials: config validation, request and response mapping, and
error classification. The shared conformance contract is
`core/history/storetest` — this module runs it rather than copying it.

An integration probe skips unless its credential environment variable is set,
so `go test ./...` is always runnable offline.

## Boundaries

This is an independent leaf module: it carries only its own SDK dependency and
never imports a sibling provider. The shared contract every module in this
family obeys is in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
