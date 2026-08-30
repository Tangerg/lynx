# luma

Package luma wraps Luma's current Agents API using the official Go SDK.
NewImageModel targets the async /v1/generations endpoint for the uni-1 family.
It submits, polls, and downloads expiring output URLs before returning
provider-neutral image bytes. Video generation is outside core/image and is not
surfaced here. See https://docs.agents.lumalabs.ai/ for the official reference.

## Install

```bash
go get github.com/Tangerg/scope/models/luma
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewImageModel`

## Testing

This module integrates a third-party service, so its tests cover what runs
without live credentials: config validation, request and response mapping, and
error classification. The shared conformance contract is `core/modeltest` for
behavior and `dev/providerconformance` for construction and API consistency —
this module runs it rather than copying it.

An integration probe skips unless its credential environment variable is set,
so `go test ./...` is always runnable offline.

## Boundaries

This is an independent leaf module: it carries only its own SDK dependency and
never imports a sibling provider. The shared contract every module in this
family obeys is in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
