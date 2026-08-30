# etl

`etl` extracts external content into `core/document.Document`, then formats,
splits, assigns IDs, batches, and writes it with explicit strategies.

Every reader produces the same intermediate form, so RAG, indexing, and
evaluation consume one document type and never learn the source format.

## Install

```bash
go get github.com/Tangerg/scope/etl
```

## Packages

| Package | Owns |
|---|---|
| `etl` | The transform and load strategies: `Formatter`, `Splitter`, `IDAssigner`, `TokenCountBatcher`, `TextFileWriter`, `SourceBudget` |
| `etl/text` | Plain-text extraction |
| `etl/json` | JSON extraction |
| `etl/markdown` | Markdown reading and structure-aware splitting |
| `etl/html` | HTML extraction (separate module — heavier parser) |
| `etl/pdf` | PDF extraction (separate module — heavier parser) |

`html` and `pdf` are separate modules because their parsers are optional and
heavy. The rest share one module because they share a dependency direction and
release cadence.

## Extract

Each reader exposes a concrete `Read` and returns Core documents. Format-specific
behavior goes in an explicit config, not a bag of options:

```go
reader, err := markdown.NewReader(source, markdown.ReaderConfig{})
if err != nil {
    return err
}
documents, err := reader.Read(ctx)
```

Metadata keys are prefixed by their reader (`markdown.heading`, …), so two
formats never collide in a downstream pipeline.

## Reading is whole-source and budgeted

Readers are built for documents that fit in memory. `SourceBudget` sets a hard
ceiling; the zero value uses a bounded default. Over the budget the read returns
`ErrSourceTooLarge` and produces **no** documents — never a silently truncated
corpus. A large source is the caller's to split or to fund with a larger budget.

## Transform

`Formatter`, `Splitter`, `IDAssigner`, and `TokenCountBatcher` each own and
validate their own policy. There is no shared fat interface across stages —
a stage's concrete type exposes its own behavior.

`markdown.Splitter` is structure-aware within a token budget: headings repeat as
retrieval context, tables split only between rows, lists only between items, and
fenced code only between lines while keeping its fences. When a single
indivisible unit cannot fit, the error names it — "table row requires N tokens;
maximum is M" — so the failure is actionable.

## Load

`TextFileWriter` writes text files. Vector-store writes stay with the
`core/vectorstore` contract and the `vectorstores/*` providers.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the invariants behind these rules.
