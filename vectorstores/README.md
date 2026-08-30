# vectorstores

`vectorstores` is the namespace holding one independent module per vector
database. Each backend implements the small capabilities it genuinely has —
`Indexer`, `Searcher`, `IDDeleter`, `FilterDeleter` — from `core/vectorstore`,
and compiles the shared filter `Predicate` into its own query dialect.

There is no aggregate module. Take only the backend you use:

```bash
go get github.com/Tangerg/scope/vectorstores/qdrant
```

The zero-dependency reference implementation is `core/vectorstore/inmemory`; it
is not part of this namespace.

## Backends

`azureaisearch`, `azurecosmos`, `bedrockkb`, `cassandra`, `chroma`,
`clickhouse`, `couchbase`, `elasticsearch`, `mariadb`, `milvus`, `mongodb`,
`neo4j`, `opensearch`, `oracle`, `pinecone`, `postgres` (pgvector and
CockroachDB), `qdrant`, `redis`, `s3vectors`, `tidb`, `typesense`, `vectara`,
`vespa`, `weaviate`.

## Asking only for what you call

A consumer depends on the narrow capability it uses, so a read-only or
delete-less backend never has to fake a method:

```go
type retriever interface {
    Search(ctx context.Context, request *vectorstore.SearchRequest) (*vectorstore.SearchResponse, error)
}
```

## Filters are provider-neutral

Write the filter once and let the backend compile it:

```go
predicate, err := filter.Parse(`category == 'tech' and year >= 2020`)
if err != nil {
    return err
}
request.Options.Filter = predicate
```

The AST is a general filter. Business identifiers travel in document metadata,
never as an AST node.

## Scores are normalized

Every database reports similarity differently. A provider interprets its raw
value and constructs a Core `Score`, so `MinScore` means the same thing whichever
backend is behind it.

## Schema initialization

Creating tables and indexes is an explicit switch. With it off, the schema is
assumed to be provisioned — a store never silently alters a backend schema, and
this module ships no migration tooling.

## Testing

These modules integrate external databases, so their tests focus on what runs
without a live server: request mapping, filter compilation, and score
normalization. Each provider also runs the public
`core/vectorstore/storetest` conformance suites, which check that capability
traversal and filter shapes succeed rather than that output matches verbatim.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the contract every backend obeys.
