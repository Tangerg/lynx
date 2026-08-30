# openai wire protocol

`protocol/openai` implements the reusable OpenAI wire adapters shared by native
and compatible provider endpoints inside the models family. It exists so several
providers that speak the same official wire share one implementation instead of
copying a request mapper and a stream accumulator each.

## Install

```bash
go get github.com/Tangerg/scope/models/protocol/openai
```

Most consumers never import this module directly. Use `models/openai` for the
native endpoint, or the provider module for a compatible one — each promotes the
shared `Model` by type alias rather than wrapping it in a forwarding shell.

## Why a separate module

A provider may depend downward on a protocol; a protocol never depends on a
provider; and providers never import each other. Keeping the wire in its own
module is what makes that direction enforceable rather than a convention.

## Testing

Tests here cover the wire itself: request and response mapping, streaming
accumulation, and error classification, all against recorded fixtures rather
than a live account. Because every compatible provider promotes this `Model`, a
regression here is a regression in all of them at once.

## Boundaries

This module owns the wire and nothing above it. Endpoint selection, credentials,
dialect quirks, and provider identity stay in the provider module.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
