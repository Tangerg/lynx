# cosmosdb

Package cosmosdb is a history Store backed by Azure Cosmos DB (NoSQL API) via
the official Azure SDK. Each message is stored as a document with a collision-
resistant random ID: { "id":              "4ZK3VZQF...", "conversation_id":
"u-42", "seq":             "1716210000000123456", "message":         "<json>",
"created_at":      "2026-05-20T08:00:00Z" } `conversation_id` is the partition
key, set when provisioning the container. Reads issue a single-partition query
and order the materialized documents by (`seq`, `id`) without requiring a
Cosmos composite index. `seq` is a fixed-width decimal string so lexicographic
ordering is numeric and Cosmos' floating-point JSON number representation
cannot lose nanosecond precision. A store-local sequence generator reserves one
contiguous range per Write and remains monotonic across local clock regression.
Concurrent calls and writes from distinct Store instances have no defined
relative order. Example: cosmos, _ := azcosmos.NewClient(endpoint, cred, nil)
container, _ := cosmos.NewContainer("scope", "chat_history") store, _ :=
cosmosdb.NewStore(cosmosdb.StoreConfig{Container: container})

## Install

```bash
go get github.com/Tangerg/scope/historystores/cosmosdb
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
