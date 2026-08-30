# neo4j

Package neo4j is a history Store backed by Neo4j via the official Go driver
(v5). Storage model: (:ChatMessage { conversation_id: "u-42", seq:
<int64 nanos>, message:         "<json>", created_at:      <datetime> }) A
composite index on (`conversation_id`, `seq`) is created by
InitializeSchema=true so reads stream in insertion order without a full
collection scan. A store-local sequence generator reserves one contiguous range
per Write and remains monotonic across local clock regression. Concurrent calls
and writes from distinct Store instances have no defined relative order.
Example: drv, _ := neo4j.NewDriverWithContext("neo4j://...", auth) defer
drv.Close(ctx) store, _ := neo4jstore.NewStore(ctx, neo4jstore.StoreConfig{
Driver:           drv, Database:         "neo4j", InitializeSchema: true, })

## Install

```bash
go get github.com/Tangerg/scope/historystores/neo4j
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
