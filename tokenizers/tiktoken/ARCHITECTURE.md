# tiktoken architecture

> The concrete tiktoken implementation of the `core/tokenizer` SPI.

Repository-wide rules live in [`../../AGENTS.md`](../../AGENTS.md); the usage
entry point is [`README.md`](README.md).

---

## 1. Position

- Adapts `github.com/pkoukk/tiktoken-go` to the small `core/tokenizer`
  interfaces, and nothing else.
- The vocabulary is selected explicitly by the caller. There is no default that
  would be correct across models.
- This module may depend on the `core/tokenizer` contract; the contract must
  never depend back on this module.

## 2. Negative invariants

- Never add a provider request, model routing, text splitting, or an application
  cache policy.
- Never use `replace` to make a release build depend on the repository layout.
