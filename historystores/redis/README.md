# redis

Package redis is a history Store backed by Redis via go-redis. Each
conversation maps to a Redis list keyed by `<KeyPrefix><conversationID>`
(default prefix `chat:history:`). Messages are RPUSH'd as canonical
chat.Message JSON, so a LRANGE 0 -1 preserves list order. When TTL is
configured, append and expiry refresh execute in one Redis transaction.
Example: client := goredis.NewUniversalClient(&goredis.UniversalOptions{...})
store, _ := redis.NewStore(redis.StoreConfig{Client: client})

## Install

```bash
go get github.com/Tangerg/scope/historystores/redis
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
