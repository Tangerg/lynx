# anthropic wire protocol architecture

> The reusable Anthropic wire, shared by every provider endpoint that speaks it
> verbatim.

The family contract is [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md);
repository-wide rules are in [`../../../AGENTS.md`](../../../AGENTS.md). The
usage entry point is [`README.md`](README.md).

---

## 1. What this module owns

- The request and response mapping between Core protocol values and the Anthropic
  wire.
- Stream accumulation: stitching SSE deltas into chunks a consumer can assemble.
- The executable `Model` types a provider promotes by type alias.

## 2. What it does not own

- Endpoint selection, base URLs, credentials, or provider identity. Those belong
  to the provider module.
- A dialect quirk only one vendor has. It travels as a typed extension from the
  provider, never as a branch in the shared wire.
- Retries and observability. The SDK retries; `otel` decorates from outside.

## 3. Dependency direction

A provider may depend on this module. This module must never depend on a
provider, and providers must never import each other. That one-way rule is why
the wire lives here instead of inside whichever provider happened to need it
first.

## 4. Read before changing

Every compatible provider promotes these types, so a change to the mapping or
the accumulator reaches all of them at once. Run the provider stream tests, not
only this module's.
