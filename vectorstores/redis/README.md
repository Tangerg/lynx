# redis

Package redis exposes Redis Stack's RediSearch module through the Core vector-
store capability interfaces. Documents are stored as Redis HASHes keyed at
`<KeyPrefix><id>`; an FT.CREATE-defined index registers the vector field plus
any pre-declared metadata fields. Requirements: Redis Stack (or Redis OSS 8.0+
with the search module) — RediSearch is mandatory. RedisJSON is NOT required;
the store deliberately uses HASH storage to keep the dependency surface
minimal. Distance metrics: DistanceCosine / DistanceL2 / DistanceIP. Vector
index algorithm: AlgorithmHNSW (default) / AlgorithmFlat. Metadata model. Every
filterable metadata key MUST be declared in StoreConfig.MetadataFields up-front
with its RediSearch type — FieldTag (exact match), FieldNumeric (range
queries), or FieldText (full-text). Filters against undeclared fields fail fast
via [ErrUnknownMetadataField] (rather than reaching Redis and silently
producing zero hits). Query path. The filter visitor emits RediSearch syntax —
TAG `@f:{v}`, NUMERIC `@f:[low high]`, TEXT `@f:(v)`. Vector retrieval runs
FT.SEARCH with the hybrid syntax `(<filter>)=>[KNN K @embedding $vec AS
distance]`, passing the binary FLOAT32 little-endian vector through PARAMS. See
https://redis.io/docs/latest/develop/interact/search-and-query/ for the
RediSearch reference.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/redis
```

## Constructors

Every constructor validates its config and returns a value implementing
the capability interfaces in `core/vectorstore`:

- `NewStore`

## Testing

This module integrates a third-party service, so its tests cover what runs
without live credentials: config validation, request and response mapping, and
error classification. The shared conformance contract is
`core/vectorstore/storetest` — this module runs it rather than copying it.

An integration probe skips unless its credential environment variable is set,
so `go test ./...` is always runnable offline.

## Boundaries

This is an independent leaf module: it carries only its own SDK dependency and
never imports a sibling provider. The shared contract every module in this
family obeys is in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
