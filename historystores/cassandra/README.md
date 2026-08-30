# cassandra

Package cassandra is a history Store backed by Apache Cassandra via gocql.
Schema (created by InitializeSchema=true): CREATE TABLE <keyspace>.<table> (
conversation_id TEXT, seq             TIMEUUID, message         TEXT, PRIMARY
KEY ((conversation_id), seq) ) WITH CLUSTERING ORDER BY (seq ASC);
`conversation_id` is the partition key and `seq` is a client-generated TIMEUUID
clustering key. Each Write reserves a strictly increasing local sequence range
and sends one unlogged batch to that partition. Concurrent calls and writes
from distinct Store instances have no defined relative order. Example: cluster
:= gocql.NewCluster("127.0.0.1") cluster.Keyspace = "scope" sess, _ :=
cluster.CreateSession() defer sess.Close() store, _ := cassandra.NewStore(ctx,
cassandra.StoreConfig{ Session:          sess, Keyspace:         "scope",
InitializeSchema: true, })

## Install

```bash
go get github.com/Tangerg/scope/historystores/cassandra
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
