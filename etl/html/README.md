# html

`html` reads HTML payloads into `core/document.Document` using
`github.com/PuerkitoBio/goquery`. It is a separate module because that parser is
optional and heavier than the base ETL dependencies.

## Install

```bash
go get github.com/Tangerg/scope/etl/html
```

## Usage

The reader extracts visible text in one of two modes:

- **Whole document** (default) — one `*document.Document` for the page.
- **Selector-scoped** — one document per element matching a CSS selector, so a
  page becomes several retrievable units.

```go
reader, err := html.NewReader(source, html.ReaderConfig{})
if err != nil {
    return err
}
documents, err := reader.Read(ctx)
```

Metadata keys carry an `html.` prefix, so they never collide with another
reader's keys in a downstream pipeline.

## Reading is whole-source and budgeted

Like every ETL reader, this one targets documents that fit in memory.
`SourceBudget` sets a hard ceiling; exceeding it returns `etl.ErrSourceTooLarge`
and produces no documents, rather than a silently truncated page.

## Boundaries

This module only extracts. Formatting, splitting, ID assignment, batching, and
loading belong to the `etl` root package.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
