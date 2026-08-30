# vespa

Package vespa exposes Yahoo Vespa's vector search through the Core vector-store
capability interfaces. Documents are regular Vespa documents in a schema with
id / content / embedding (tensor) fields plus any metadata attributes — reached
over the HTTP Document / Search REST APIs. Requirements: a Vespa application
(Vespa Cloud or self-hosted) with a schema (.sd file) declaring the embedding
tensor field and any metadata attributes the filter visitor will address. The
store does NOT create the schema — Vespa schemas are part of the application
package, not a runtime API. Authentication. Talk to Vespa over HTTPS with mTLS
(Vespa Cloud) or plain HTTP (self-hosted). Inject credentials by passing a
configured http.Client via StoreConfig.HTTPClient. Search shape. The store
issues a `nearestNeighbor` YQL search: POST /search/ { "yql": "select * from
<schema> where {targetHits:K}nearestNeighbor(<vec_field>, q) and <filter>",
"hits": K, "input.query(q)": {"values": [...]}, "ranking": "default" } The
result's `relevance` is taken as-is — for the default cosine configuration this
is already a [0, 1] similarity score. Filter visitor produces YQL where-clause
fragments — `author contains "Alice"` (equality on string fields uses
`contains`), `year >= 2020`, `tag in ("a", "b")`, `!(...)`, ` and ` / ` or `.
Every filterable metadata key must exist as a top-level attribute in the
schema. Delete. Vespa selection expressions live under their own mini language;
rather than translate the AST a second way, the store enumerates ids via a YQL
search and then issues per-id deletes against the Document API (`DELETE
/document/v1/<ns>/<schema>/docid/<id>`). See https://docs.vespa.ai/en/nearest-
neighbor-search.html.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/vespa
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
