# Bug reports

Each report records one reproducible defect, its owning layer, and its current
resolution. The report stays only while that diagnosis remains useful for
preventing the same mistake.

## File format

`NNN-short-slug.md`, with:

- **Module / package** and the symbol involved.
- **Observed behavior**: what the code does now, with the input that shows it.
- **Expected behavior**: the contract it comes from (GoDoc, a checked example,
  an executable architecture guard, or a protocol rule).
- **Blast radius**: who consumes the symbol, and whether a fix is a breaking
  public API change.
- **Suggested fix layer**: the root cause's layer, not the symptom's.

## Current status

| Report | Status |
|---|---|
| [001 — testActiveChildLimit deadlocks the whole agent package](001-active-child-limit-test-deadlock.md) | Fixed in the test; no implementation change needed |
