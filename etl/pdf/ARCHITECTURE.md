# pdf architecture

> PDF text extraction into Core documents. A separate module because its parser
> is optional and heavy.

The ETL contract is [`../ARCHITECTURE.md`](../ARCHITECTURE.md); repository-wide
rules are in [`../../CLAUDE.md`](../../CLAUDE.md). The usage entry point is
[`README.md`](README.md).

---

## 1. What this module owns

- A validated `ReaderConfig` and the reader it produces.
- Per-page plain-text extraction, emitted whole or per page.
- Its own `pdf.`-prefixed metadata keys.

## 2. What it does not own

- Rendering, OCR, or layout reconstruction. This reader reports what the parser
  extracts and does not guess at the rest.
- Transform and load. Those belong to the `etl` root package.
- Streaming extraction. This is a whole-source reader bounded by
  `etl.SourceBudget`.

## 3. Dependency island

The PDF parser dependency stays here. It must never reach the base `etl` module,
which is why this is a separate leaf module rather than a subpackage.
