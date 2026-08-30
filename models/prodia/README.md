# prodia

Package prodia wraps Prodia's image generation REST API. NewImageModel targets
the synchronous POST /v2/job endpoint for text-to-image job types.
image.Options.Model is the official job type discriminator, such as
"inference.flux-fast.schnell.txt2img.v2"; the type-specific config is available
through [JobRequest] under ImageRequestExtensionKey. PNG, JPEG, and WebP output
use the endpoint's standard Accept negotiation. See
https://docs.prodia.com/reference/inference/ for the full reference.

## Install

```bash
go get github.com/Tangerg/scope/models/prodia
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
