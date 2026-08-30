# core architecture

> The narrow waist of Scope: only the protocols, the minimal calling SPI, and
> the pure composition that every provider shares.

This document is the design contract for the `core` module. Repository-wide
rules live in [`AGENTS.md`](../AGENTS.md),
[`DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md), and
[`REFACTORING.md`](../REFACTORING.md); the usage entry point is
[`README.md`](README.md).

When the code and this document disagree, neither side wins by default: fix the
implementation if it is wrong, and update this document together with the code
if reality has overtaken the design.

---

## 1. Position

- **The module is the release boundary; the package is the responsibility
  boundary.** Core owns metadata, media, and document values, the request and
  response protocol of each modality, the minimal `Model` capability, and the
  high-level vector-store semantics. `chatclient`, `embeddingclient`,
  `jsonschema`, `tool`, `tokenizer`, `history`, and the in-memory reference
  implementations share Core's dependency direction and release cadence, so they
  live in this module — but they stay separate packages and never merge into one
  general framework.
- **Production dependencies obey the architecture boundary.** The standard
  library comes first; where it falls short, an evaluated, semantically precise,
  well-maintained general library is allowed. Core never imports a sibling
  module, a provider SDK, a concrete tokenizer vocabulary, or OpenTelemetry. The
  architecture gates check dependency direction and public API leakage rather
  than enforcing an exact third-party allowlist, and a temporary wrapper is not
  a way around the boundary.
- **The dependency direction is one-way.** Providers, capability modules, the
  agent kernel, and the OTel adapters may import Core. Core imports none of
  them.

## 2. Mental model

- **Flat, partitioned by meaning.** Protocol packages use domain names —
  `chat`, `embedding`, `image`. The conveniences that sit next to a protocol
  live in `chatclient` and `embeddingclient`. Cross-protocol capabilities live
  in `jsonschema`, `tool`, `tokenizer`, `history`, and `vectorstore`. Directory
  depth expresses semantics or ownership, never decoration.
- **Minimal capability interfaces.** Each modality's `Model` has only `Call` by
  default. Real streaming is expressed as an independent `Streamer`, and
  dimension probing that requires a request belongs to the consuming workflow
  rather than masquerading as a Core SPI.
- **One error vocabulary.** Every provider-facing modality package exports
  `ErrInvalidOptions`, `ErrInvalidRequest`, and `ErrInvalidResponse` as the
  stable classification shared by providers and integrations. Chat may add its
  own protocol errors on top of that triple. The `internal/arch` modality gate
  enforces this and derives its package list from the packages that publish a
  `Model` SPI, so a new modality joins by existing rather than by being
  remembered.
- **Protocol values are serializable.** A DTO never carries a closure, reader,
  logger, tracer, registry, native client, or any other runtime object. Metadata
  and extensions must stay JSON-safe and are validated explicitly at every I/O
  boundary.
- **Pointers express presence, not anemia.** A pointer field in a wire DTO
  distinguishes "absent" from "explicitly zero", or marks the optional branch of
  a tagged payload. An optional pointer must carry `omitempty` or `omitzero`; a
  required nested object pointer must be rejected as nil by its owner's
  `Validate`. `internal/arch` scans the public JSON DTOs to enforce this.
- **Tagged values, not a sealed hierarchy.** Messages and parts use a public
  discriminator and ordinary values. An unknown type returns a diagnosable error
  instead of relying on an unexported method to seal the type set.
- **Streaming is `iter.Seq2`.** No custom iterator, and no channel pretending to
  be a pull model. Early caller stop, context cancellation, and first-error
  termination all require tests.
- **Extension mechanisms belong to their owner.** Protocol extension goes
  through typed metadata; model-call behavior is composed from clients and
  middleware; history, safeguard, and the in-memory reference implementations
  stay in their semantic package; OTel and provider policy stay in external
  modules.
- **Contract tests belong to the contract owner.** `modeltest`,
  `history/storetest`, and `vectorstore/storetest` supply reusable suites and
  fixtures only. They contain no provider implementation and never become a
  dependency of a production adapter.

## 3. Subsystem contracts

### 3.1 `tool` — the executable-tool waist

- **Contract, generic adaptation, and the instance set.** `Tool`, `Binding`,
  `Authorizer`, `Guard`, `WrappingTool`, `Capability`, `Func`, `NewFunc`, and
  `Registry` belong to this package. Schema derivation, parsing, and validation
  belong to `jsonschema` alone. Shell, filesystem, HTTP, search, and skill
  execution belong to the `tools` module family.
- `Tool` expresses only `Definition` and `Call`. It never absorbs retries,
  concurrency, human-in-the-loop, checkpointing, or a provider protocol.
- `Guard` asks an `Authorizer` only at the execution boundary of an already
  validated `Invocation`. Identity, tenancy, consent flows, and policy storage
  are held by the caller through the context and a concrete implementation, and
  never enter Core.
- `NewFunc` derives a strict schema from the input type and owns the strict JSON
  codec of a typed function.
- Optional capabilities are defined by the consumer and discovered along the
  `WrappingTool` chain through `Capability`; the outermost implementation wins.
- **No global state.** Every runtime constructs and owns its own `Registry`.

### 3.2 `tokenizer` — a pure capability SPI

- Defines only `TextEstimator`, `Encoder`, `Decoder`, and their minimal
  composition `Tokenizer`.
- Depends on the standard library alone: the contract imports no tiktoken, no
  provider SDK, and no sibling module.
- The interface follows consumption semantics: a provider that only counts
  tokens is never forced to pose as a reversible encoder. Concrete vocabularies
  and third-party implementations live in the `tokenizers/<provider>` leaf
  modules.

### 3.3 `vectorstore` — application semantics preserved

- The public surface still deals in documents and query text, but is split into
  the small `Indexer`, `Searcher`, `IDDeleter`, and `FilterDeleter` capabilities.
- `IndexRequest` owns its indexing input and batching rules. `SearchRequest`,
  `SearchOptions`, `SearchResult`, and `SearchResponse` each own their own
  invariants, and `Score` unifies the provider boundary.
- `filter` publishes only an immutable AST, the visitor contract, and the
  semantic methods on its nodes. The scanner, analyzer, formatter, and optimizer
  stay private operation objects.

## 4. Evolution discipline

- Removed packages, aliases, bridges, and generic frameworks are never
  reintroduced.
- A breaking change migrates every workspace consumer in the same batch. No
  deprecated wrapper, dual read/write, or legacy wire decoding survives it.
- An exported API change must account for the provider and backend blast radius
  and update package docs, examples, and current-behavior tests together. During
  development the repository keeps no release baseline, old API snapshot, or
  release notes.
- JSON DTOs, tags, and omission rules are verified by round-trip,
  malformed-input, omitempty, and provider conformance tests owned by the
  package that declares them. There is no global cross-package wire snapshot.

## 5. Negative invariants

- Never place a provider SDK, an external storage backend, a concrete tokenizer
  vocabulary, a business tool executor, agent control flow, evaluation, or OTel
  instrumentation inside Core.
- Never simulate inheritance with a generic `Model`/`StreamingModel`, and never
  force `Model` to provide default options, metadata, or streaming.
- Never put `any`, a closure, an SDK client, or an `io.Reader` into a wire DTO.
- Never add a global registry, cache, or state, and never let a probe failure
  return silently as zero or empty.
- Never promote an option or taxonomy that only one provider supports, or that
  means different things across providers, into a fixed Core field.
- Never add a second advisor, hook, interceptor, or plugin chain.
- Never replace `iter.Seq2` streaming with channels.
- Never let `tool` import `tools`, `agent`, `mcp`, `a2a`, a provider SDK, or an
  application; never add concrete tool factories, re-export schema derivation as
  forwarding API, bypass `jsonschema` to reach a third-party schema library, or
  introduce global registration or auto-discovery.
- Never push runtime policy into the `Tool` interface — a new interface method
  breaks every external implementation.
- Never place a vocabulary, model-name mapping, cache, default encoding, or
  third-party implementation into `tokenizer`, and never grow its small
  interfaces for convenience, which would force a count-only, encode-only, or
  decode-only implementation to carry unrelated capability.

## 6. Read before changing

- A change to `Message`, `Request`, or `Response` reaches every chat provider and
  several agent, RAG, and tool consumers.
- A change on the document or vector-store path must cover ETL, RAG, and every
  `vectorstores` backend.
- A change to the public filter surface must be mirrored in every backend
  visitor; the lexer, parser, token, and visitor internals must not become new
  external dependencies.

## 7. Gates

Architecture invariants are held by executable gates, not by review memory:

| Gate | Location | Covers |
|---|---|---|
| Dependency direction, public API leakage, modality inventory, error vocabulary | `internal/arch` | All of Core |
| Wire DTO pointer presence semantics | `internal/arch` | Public JSON DTOs |
| Per-package boundaries for chatclient, embeddingclient, tokenizer, tool | `<package>/internal/arch` | The owning package |
| Per-package statement coverage budget | `scripts/check-core-coverage.sh` | All of Core, including empty packages |
| Cross-module architecture and documentation | `dev/repoarch` | The whole repository |
