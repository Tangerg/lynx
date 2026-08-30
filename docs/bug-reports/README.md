# Bug reports

One file per defect found while raising test coverage, so implementation fixes
can be batched and reviewed separately from the tests that exposed them.

Tests in the coverage pass are written against current behavior. When a test
cannot be written without asserting something wrong, the defect is filed here
instead of being fixed inline, and the test is either omitted or written to the
correct behavior and left failing only if the report says so explicitly.

## File format

`NNN-short-slug.md`, with:

- **Module / package** and the symbol involved.
- **Observed behavior** — what the code does now, with the input that shows it.
- **Expected behavior** — and the contract it comes from (godoc, ARCHITECTURE.md,
  or a protocol rule).
- **Blast radius** — who consumes the symbol, and whether a fix is a breaking
  public API change.
- **Suggested fix layer** — the root cause's layer, not the symptom's.

## Current status

| Report | Status |
|---|---|
| [001 — testActiveChildLimit deadlocks the whole agent package](001-active-child-limit-test-deadlock.md) | Fixed in the test; no implementation change needed |

No implementation defect has been found. Every gap closed in this pass was a
missing test, not wrong behavior: the Core protocol wire paths, the MCP content
mappings in both directions, the skills confinement and precedence rules, the
web freshness mappings, the tools concurrency declarations, the OTel error
classifications, the ETL extraction boundaries, the RAG identity stages, the
eval report boundaries, and the agent vocabulary and planning contracts all
matched their documented behavior once exercised.

One test defect was found and is filed as 001: a subtest waited on a dispatch
that races against parent cancellation, so it deadlocked the whole package under
`GOMAXPROCS=1`. The kernel behaved exactly as ARCHITECTURE.md specifies; the test
relied on the race the architecture forbids relying on.

One architecture violation was found and fixed at the source rather than filed:
adding `core/doc.go` for a module overview created a Go package at a namespace
root, which `dev/repoarch` forbids because it would be the root façade the
architecture rules out. The gate caught it, the overview moved to
`core/README.md`, and no product code changed.

Three things were confirmed as intentional rather than filed:

- Several `Options.applyOverride` and `Validate` branches in the Core modality
  packages are unreachable from the public API, because `metadata.Extensions`
  cannot be constructed in an invalid state. They are defensive, not dead.
- The `evalLogical`, `evalUnary`, and `Visit` type-mismatch branches in
  `core/vectorstore/inmemory` are unreachable through `filter`'s constructors
  and parser, which only produce boolean predicates. They guard against a future
  node type rather than a current input.
- The Markdown splitter does not fail on an oversized single word. Prose has no
  indivisible unit above the word, so a long paragraph falls back to token
  windows by design; only a structural unit — a table row, a list item, a code
  line — can be reported as too large.
