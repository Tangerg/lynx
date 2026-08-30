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

No implementation defect has been found so far. Every gap closed in this pass
was a missing test, not wrong behavior: the protocol wire paths, the MCP content
mappings, the skills confinement rules, the web freshness mappings, and the
tools concurrency declarations all matched their documented contracts once
exercised.

Two things were confirmed as intentional rather than filed:

- Several `Options.applyOverride` and `Validate` branches in the Core modality
  packages are unreachable from the public API, because `metadata.Extensions`
  cannot be constructed in an invalid state. They are defensive, not dead.
- The `evalLogical`, `evalUnary`, and `Visit` type-mismatch branches in
  `core/vectorstore/inmemory` are unreachable through `filter`'s constructors
  and parser, which only produce boolean predicates. They guard against a future
  node type rather than a current input.
