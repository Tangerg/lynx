# rag architecture

> Small interfaces plus composition functions. There is no fixed pipeline: a
> caller uses `Retriever` as the narrow waist and assembles exactly the
> capability it needs.

Repository-wide rules live in [`../AGENTS.md`](../AGENTS.md). Contracts,
adapters, and dependency versions follow the code; this document states the
boundaries only.

---

## 1. Position

- **RAG is a free composition of small interfaces, not a framework pipeline.**
  Each contract — transform, expand, retrieve, refine, augment — is one small
  interface, and a caller assembles them with ordinary Go composition
  functions.
- **Adapters are exposed from the root package under concrete names.** Anything
  in the same RAG domain starts in the root package; the package is not split
  ahead of a real structural need.

## 2. Mental model

- **`Retriever` is the narrow waist.** Capability is expressed by composing
  around it — layering a transformer, an expander, a refiner — rather than by
  describing a whole pipeline in one large config object.
- **Composition uses functions, not framework configuration.** There is no
  `PipelineConfig` or `Pipeline` central object.
- **One package first.** The RAG domain stays in the root package with concrete
  type names, rather than pre-splitting into `rag/vectorstore`, `rag/llm`, and
  the like.
- **Only fan-out retrieval runs in parallel.** Multi-route retrieval and query
  expansion collect concurrently; transform and refine are explicit sequential
  steps. Concurrent results merge in declaration order, and a failing branch
  fails the retrieval — an incomplete result must never be presented as a
  complete hit set.
- **One document identity takes one retrieval slot.** For the same non-empty
  document ID the highest-scoring candidate wins, ties broken stably by first
  appearance. That uniqueness happens before `TopK` truncates, so refiner order
  cannot decide correctness.
- **Per-call query metadata travels as typed `ValueKey` values.** Filters,
  history, and tenant context ride an immutable `Query` envelope. The public API
  exposes no string-keyed `any` map, and a same-name different-type collision is
  an explicit error. A reference-typed value stays owned by the caller and must
  be read-only during parallel retrieval.
- **Evaluation is a peer, not a part.** General evaluation lives in the top-level
  `eval` module. RAG produces evaluable inputs; it does not own an evaluation
  framework.
- **Observation belongs to the integration.** RAG propagates `context.Context`
  and never imports OpenTelemetry. A composition root decorates the final
  `Retriever` — or a branch that needs separate attribution — with `otel/rag`.
- **Reranking has one refinement boundary.** `Reranker` adapts the dedicated
  Core rerank protocol, while `ChatReranker` is the explicitly generative
  implementation. Both reduce to `Refiner`; neither introduces another
  retrieval runtime or provider dependency.

## 3. Negative invariants

- Never reintroduce `PipelineConfig` or `Pipeline`. Composition is done with Go
  functions.
- Never add a fixed stage such as `QueryRouter` or `DocumentJoiner`. Routing is a
  custom `Retriever`; merging is a `Refiner`.
- Never split the root package back into `rag/vectorstore`, `rag/llm`,
  `rag/ragchat`, or `rag/eval`. One package with concrete names is enough;
  cross-domain capability becomes a peer module.
- Never grow a large `Config` or a builder for a capability. Small interfaces
  plus function composition come first, and only genuine options enter a config.
- Never let a duplicate candidate consume a `TopK` slot or keep a lower-scoring
  first hit. Merging several retrievers must keep the highest score per ID, and
  the order in which `Dedup` and `TopK` compose must not change the unique top
  set.
- Never degrade a nil or empty output silently into identity. An optional
  capability uses an explicit identity or no-op implementation; a composer
  rejects nil at construction, and an empty model output, an empty expansion, or
  a concurrent branch error must be returned.

## 4. Read before changing

- Adding a capability: first ask whether it belongs to the RAG domain. If it
  does, it goes in the root package — unless it is clearly an independent
  lower-level library.
- Adding a concrete adapter: an ordinary struct with a concrete constructor
  name. Only genuine options enter a config.
