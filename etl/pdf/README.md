# pdf

`pdf` reads PDF payloads into `core/document.Document` using
`github.com/ledongthuc/pdf`, a pure-Go parser forked from `rsc/pdf`. It is a
separate module because that parser is optional and heavier than the base ETL
dependencies.

## Install

```bash
go get github.com/Tangerg/scope/etl/pdf
```

## Usage

The reader extracts plain text from each page in one of two emission modes:

- **Whole document** — one `*document.Document` for the file.
- **Per page** — one document per page, so page numbers survive into retrieval.

```go
reader, err := pdf.NewReader(source, pdf.ReaderConfig{})
if err != nil {
    return err
}
documents, err := reader.Read(ctx)
```

Metadata keys carry a `pdf.` prefix, so they never collide with another reader's
keys in a downstream pipeline.

## Reading is whole-source and budgeted

Like every ETL reader, this one targets documents that fit in memory.
`SourceBudget` sets a hard ceiling; exceeding it returns `etl.ErrSourceTooLarge`
and produces no documents, rather than a partially parsed file.

## Boundaries

This module only extracts text. It renders nothing, performs no OCR, and does
not interpret layout beyond what the parser reports.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
