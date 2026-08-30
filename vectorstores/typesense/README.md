# typesense

Package typesense exposes Typesense's vector search through the Core vector-
store capability interfaces. Documents are regular Typesense documents in a
collection with id / content / metadata (nested object) / embedding (float[])
fields, reached through the official typesense-go v3 client. Requirements:
Typesense 0.25+ (vector search GA) — the store uses nested-object metadata
which needs `enable_nested_fields=true` on the collection. Distance metric:
cosine only. Typesense's vector search always uses cosine distance — the result
`vector_distance` is in [0, 2] and the store maps it onto a higher-is-better
score in [0, 1]. Schema bootstrap. When StoreConfig.InitializeSchema is true
the store probes for the collection and creates it with the right fields +
dimensionality if missing. Existing collections are trusted as-is. Filter
visitor produces Typesense `filter_by` syntax — `metadata.k:= v`,
`metadata.year:>= 2020`, `metadata.tag:= [a,b]` (IN form). The metadata field
is a nested object so keys are addressed under the configured prefix. NOT
caveat. Typesense `filter_by` has no top-level NOT operator — the visitor
rewrites `NOT (x op y)` into the operator's inverse (e.g. `NOT (year >= 2020)`
→ `metadata.year:< 2020`). NOT wrapping anything other than a single binary
comparison is rejected. See https://typesense.org/docs/latest/api/vector-
search.html.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/typesense
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
