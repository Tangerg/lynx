# html architecture

> HTML extraction into Core documents. A separate module because its parser is
> optional and heavy.

The ETL contract is [`../ARCHITECTURE.md`](../ARCHITECTURE.md); repository-wide
rules are in [`../../AGENTS.md`](../../AGENTS.md). The usage entry point is
[`README.md`](README.md).

---

## 1. What this module owns

- A validated `ReaderConfig` and the reader it produces.
- Extraction of visible text, in whole-document or selector-scoped mode.
- Its own `html.`-prefixed metadata keys.

## 2. What it does not own

- Transform and load. `Formatter`, `Splitter`, `IDAssigner`,
  `TokenCountBatcher`, and the writers live in the `etl` root package.
- A general `Reader` interface. Core does not own an interface with no consumer;
  this reader exposes a concrete `Read`.
- Streaming extraction. This is a whole-source reader bounded by
  `etl.SourceBudget`; a large document is the caller's to split or to fund.

## 3. Dependency island

The goquery dependency stays here. It must never reach the base `etl` module,
which is why this is a separate leaf module rather than a subpackage.
