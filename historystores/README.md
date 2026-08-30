# historystores

`historystores` is the namespace holding one independent module per external
conversation-history backend. The contract, the middleware, and the in-memory
reference implementation live in `core/history`; these modules only persist.

There is no aggregate module. Take only the backend you use:

```bash
go get github.com/Tangerg/scope/historystores/postgres
```

## Backends

`cassandra`, `cosmosdb`, `mongodb`, `neo4j`, `postgres`, `redis`.

## Asking only for what you call

`core/history` splits its store into small interfaces, so a consumer that only
reads does not depend on a writer:

```go
type reader interface {
    Read(ctx context.Context, id history.ConversationID) ([]chat.Message, error)
}
```

## One wire, partitioned by conversation

Every backend reads and writes the current `core/chat.Message` tagged wire — one
canonical envelope, no compatibility branches. Storage is partitioned by
conversation ID so no operation scans across conversations, and ordering comes
from a monotonic sequence or list append rather than a timestamp, which is not
stable under concurrency.

## Schema initialization

Creating tables is an explicit switch, normally off in production where
migration happens ahead of time. A custom table name must pass SQL identifier
validation — that is the injection trust boundary, and this module ships no
migration tooling.

## Testing

These modules integrate external databases, so their tests focus on what runs
without a live server: envelope round-trips, identifier validation, and query
construction. The shared conformance suite is `core/history/storetest`.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the contract every backend obeys.
