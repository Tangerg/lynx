# vectorstores architecture

> One adaptation layer for every vector-database backend. Each backend
> implements the `Indexer`, `Searcher`, `IDDeleter`, and `FilterDeleter`
> capabilities it genuinely has, and compiles the stable filter `Predicate` into
> its own query dialect.

Repository-wide rules live in [`../CLAUDE.md`](../CLAUDE.md). The backend
inventory, maturity, and dependency versions follow the code; this document
states the shared contract every `vectorstores/<backend>` module obeys. The
usage entry point is [`README.md`](README.md).

---

## 1. Position

- **A set of small capability interfaces, many database backends.** A consumer
  asks only for the capability it calls. No fat `Store` forces a read-only or
  delete-less backend to fake a method.
- **Adding a backend means implementing its real capabilities and writing a
  filter compiler.** The shared filter semantics never change for one vendor.
- **One external backend, one leaf module.** `vectorstores/<backend>` carries
  only its own implementation, and backends never import each other. The
  PostgreSQL family shares `vectorstores/postgres` because it shares one
  pgwire/pgvector stack, while keeping `pgvector` and `cockroachdb` as separate
  public packages. The zero-dependency reference implementation lives in
  `core/vectorstore/inmemory` and is not part of this namespace.

## 2. Mental model

- **The public filter surface expresses semantics only.** `Predicate`,
  `Selector`, and `Parse` are the stable entry points; the package-private
  scanner, tokens, and recursive-descent parser build the one AST directly. A
  backend-private compiler implements `filter.Visitor`, and the store translates
  that one semantic tree into its dialect — a JSONB path, a flat dictionary, a
  nested query. A provider never publishes a second compiler API. A genuinely
  external implementation still plugs in its own interpreter through
  `Predicate.Accept`.
- **Two things per backend:** the backend implementing its capability set, and
  the compiler turning a `Predicate` into the dialect. Ingestion, filter, and
  score semantics that are stable across providers belong to Core; provider
  identifiers, schema, and wire encoding stay in the provider package; the
  shared test contract belongs to its owner, `core/vectorstore/storetest`.
  Cross-cutting concerns such as OTel are supplied by an outer decorator and
  never intrude into a provider.
- **Vector encoding and distance metrics differ per database.** A provider
  interprets the raw value and then constructs Core's `Score`, so a consumer
  always receives a score in one range.
- **Schema initialization is an explicit switch.** On, it creates tables and
  indexes; off, it assumes the schema is already provisioned. It never silently
  runs `ALTER`.
- **Bulk upsert splits at two levels.** The caller injects a batcher to control
  embedding batches, and the backend splits again against its own API limit.
- **A writable store always persists the body.** `Searcher` must return a
  complete, verifiable `Document` that can go straight into RAG. There is no
  switch producing ID-only results. An external document-repository pattern is
  composed explicitly through hydration instead.
- **Conformance coverage comes free.** Each provider package runs the public
  `core/vectorstore/storetest` capability and filter-shape suites, which verify
  that traversal succeeds rather than that output matches verbatim.

## 3. Negative invariants

- Never build a cross-backend data migration tool. That is operations work, not
  an SDK responsibility.
- Never add a business concept — a conversation ID, say — as a filter AST node.
  The AST is a general filter; business fields travel in metadata.
- Never reshape vector dimensions on the store side. Dimensions are negotiated
  through config and the embedding model, not adjusted on write.
- Never change a backend schema silently. With initialization off, the schema is
  assumed to exist.
- Never bundle unrelated backends with an aggregate `go.mod` or a `replace`. An
  optional database SDK stays on its own dependency island, in-repository
  cooperation happens only through `go.work`, and only provider packages inside
  one indivisible stack may share a module-internal engine.

## 4. Read before changing

- Changing Core's filter AST or visitor forces every backend compiler to follow.
  Run the shared conformance suite first.
- Changing a capability interface is a Core contract change: the blast radius is
  every backend implementing it plus its consumers, such as `rag`.
- Adding a backend: take the contract semantics from
  `core/vectorstore/inmemory`, implement only the real capabilities in an
  independent module, and run the conformance suite.
