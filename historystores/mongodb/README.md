# mongodb

Package mongodb is a history Store backed by MongoDB via the official mongo-
driver v2. Each message is a document in the configured collection: { "_id":
ObjectId(...),     // assigned by the driver "conversation_id": "u-42", "seq":
1716210000000123456, "message":         "<json>",          // canonical
chat.Message wire shape "created_at":      ISODate(...), } Documents are read
by (`seq`, `_id`). A store-local sequence generator reserves one contiguous
range per Write and remains monotonic across local clock regression. Concurrent
calls and writes from distinct Store instances have no defined relative order.
Example: col := client.Database("scope").Collection("chat_history") store, _ :=
mongodb.NewStore(ctx, mongodb.StoreConfig{ Collection:       col,
InitializeSchema: true, // create the conversation_id index })

## Install

```bash
go get github.com/Tangerg/scope/historystores/mongodb
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
