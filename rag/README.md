# rag

`rag` provides small interfaces and combinators for Retrieval-Augmented
Generation. There is no pipeline object and no central config: `Retriever` is
the narrow waist, and you wrap it with exactly the stages you need.

## Install

```bash
go get github.com/Tangerg/scope/rag
```

## Quick start

```go
query, err := rag.NewQuery("what is GOAP?")
if err != nil {
    return err
}

documents, err := rag.Retrieve(ctx, retriever, query)
```

## Composition is explicit

Five contracts — `Transformer`, `Expander`, `Retriever`, `Refiner`, and
`Augmenter` — compose with ordinary functions:

```go
retriever, err := rag.WithTransformers(base, rewrite, translate)
retriever, err = rag.WithExpander(retriever, multiQuery)

topK, err := rag.TopK(8)
retriever, err = rag.WithRefiners(retriever, topK)

documents, err := rag.Retrieve(ctx, retriever, query)
```

An optional stage uses an explicit identity implementation —
`IdentityTransformer`, `IdentityExpander`, `NopRetriever`, `IdentityRefiner`,
`IdentityAugmenter` — never a silently skipped nil.

## Reranking

`Reranker` adapts a dedicated `core/rerank.Model` into the `Refiner` lifecycle.
It formats each candidate once, sends one immutable indexed batch, then resolves
the returned indices to independently owned candidates. `ChatReranker` is the
separate generative implementation for a chat model with structured output;
its name makes the different cost and failure semantics explicit.

## Semantic and hybrid retrieval

`VectorStoreRetrieverConfig.SearchMode` forwards Core's one retrieval-strategy
contract. Its zero value is semantic. Hybrid asks a supporting backend to
combine lexical and semantic evidence natively; an unsupported backend returns
`vectorstore.ErrUnsupportedSearchMode` instead of silently falling back.
`MinScore` is unavailable in hybrid mode because fused score scales are not
portable across backend algorithms.

## What runs in parallel

Only fan-out: multi-route retrieval and query expansion collect concurrently.
Transform and refine are sequential steps you can read in order.

Concurrent results merge in declaration order, so a run is reproducible. If any
branch fails, the retrieval fails — an incomplete hit set is never returned as
if it were complete.

## Deduplication comes before TopK

The same non-empty document ID takes exactly one slot: the highest-scoring
candidate wins, ties broken stably by first appearance. Uniqueness is settled
before truncation, so the order in which you compose `Dedup` and `TopK` cannot
change the top set.

## Per-call context

Filters, conversation history, and tenant context ride an immutable `Query`
envelope through typed `ValueKey` values. There is no string-keyed `any` map on
the public API, and a same-name different-type collision is an error rather than
a silent overwrite. A reference-typed value stays owned by the caller and must
be treated as read-only while parallel retrieval is in flight.

## What this module does not own

- **Evaluation.** The `eval` module owns that. RAG produces evaluable inputs.
- **Observability.** RAG propagates `context.Context` only. A composition root
  decorates the final retriever with `otel/rag`.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the invariants behind these rules.
