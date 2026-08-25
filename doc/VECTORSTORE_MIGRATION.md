# Vectorstore breaking migration

`core/vectorstore` now models indexing and search as complete operations. This
is an intentional breaking change; old procedural helpers and compatibility
aliases were removed.

## Indexing

Replace `Indexer.Add(ctx, documents)` with an `IndexRequest`:

```go
request, err := vectorstore.NewIndexRequest(documents)
if err != nil {
	return err
}
if err := indexer.Index(ctx, request); err != nil {
	return err
}
```

`IndexRequest.Validate` owns document validation. `IndexRequest.Batch` owns
batch-policy execution and returns validated, order-preserving child requests.

## Search

Search policy moved under `SearchOptions`, and search returns one response with
`Results`:

```go
request := &vectorstore.SearchRequest{
	Query: "attention",
	Options: vectorstore.SearchOptions{
		TopK:     5,
		MinScore: 0.7,
		Filter:   predicate,
	},
}
response, err := searcher.Search(ctx, request)
```

Each `SearchResult` owns one document/score relation. `SearchResponse.Validate`
checks intrinsic result invariants; `SearchResponse.ValidateFor` additionally
checks the request's threshold and TopK policy.

All vectorstore wire models now validate during JSON marshal/unmarshal. Errors
can be classified with `ErrInvalidOptions`, `ErrInvalidRequest`,
`ErrInvalidResponse`, and `ErrInvalidScore`.

## Scores and filters

Provider score conversion now returns the `vectorstore.Score` value object.
Use its `Float64` method only at an SDK boundary.

Filter trees are immutable rich models. Use node methods such as `Path`,
`Value`, `Values`, `Pattern`, `Negated`, and `Dispatch`; pass a compiler or
interpreter through `Predicate.Accept`. The former package-level validation,
extraction, conversion, formatting, and dispatch helpers no longer exist.
