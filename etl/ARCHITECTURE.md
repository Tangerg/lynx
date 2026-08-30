# etl architecture

> Extract external content into `core/document.Document`, then format, split,
> assign IDs, batch, and load it with explicit strategies.

Repository-wide rules live in [`../AGENTS.md`](../AGENTS.md). Parser libraries
and versions follow each module's `go.mod`; the usage entry point is
[`README.md`](README.md).

---

## 1. Position

- **ETL is a complete capability domain, not a reader namespace.** The root
  package owns the general transform and load strategies; `text`, `json`,
  `markdown`, `html`, and `pdf` each own extraction for their format.
- **One intermediate form.** Every reader produces a Core `Document`. RAG,
  indexing, and evaluation consume that one type and never learn the source
  format.
- **Modules isolate real dependency boundaries only.** The lightweight `text`
  and `json` packages and `markdown` — which shares reader and splitter
  responsibility — live in the base module. The heavier optional parsers for
  HTML and PDF stay in their own leaf modules.

## 2. Mental model

- **Extract.** Each reader exposes a concrete `Read(context.Context)` returning
  Core documents, and expresses format-specific behavior through an explicit
  `Config`. Core does not own a general `Reader` interface that has no consumer.
- **Transform.** `Formatter`, `Splitter`, `IDAssigner`, and `TokenCountBatcher`
  each own and validate their own policy. Configuration never arrives as a magic
  map or a one-shot option closure.
- **Load.** `TextFileWriter` names a text-file target explicitly. Vector-store
  writes belong to the `core/vectorstore` contract and the `vectorstores/*`
  providers.
- **Metadata keys carry a reader prefix**, so each format lands in its own
  namespace and two readers never collide.
- **Whole-source reads are budgeted, never implicitly truncated.** These readers
  target small documents; `SourceBudget` sets a hard ceiling, the zero value
  uses a bounded default, and exceeding it returns `ErrSourceTooLarge` with no
  partial documents. A large document must be split by the caller or funded with
  a larger budget.
- **Markdown has one owner.** Reading and structure-aware splitting share
  `etl/markdown`; two modules never own the same format parser.

## 3. Negative invariants

- Never invent a unified fat interface per processing stage. An interface is
  owned by its real consumer; a concrete type exposes its own rich behavior.
- Never add a `go.mod` to a lightweight subpackage. A module is a dependency and
  release boundary, not directory decoration.
- Never let a heavy-dependency reader pollute the base module. Optional parsers
  such as HTML and PDF stay independent leaf modules.
- Never omit the reader prefix from a metadata key. It would collide with
  another reader and corrupt a downstream key lookup.

## 4. Read before changing

- Renaming a metadata key: a downstream RAG pipeline may read it directly.
  Coordinate across readers before changing one.
- Adding a format: create a subpackage returning Core documents with
  format-prefixed metadata. Create a leaf module only when dependency weight or
  an independent release cadence forms a real boundary.
