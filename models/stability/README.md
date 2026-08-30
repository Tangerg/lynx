# stability

Package stability wraps Stability AI's image generation REST API. NewImageModel
targets the v2beta /stable-image/generate endpoints (ultra, core, sd3, etc.).
Per-model knobs (aspect_ratio, negative_prompt, seed, style_preset,
output_format) ride through the typed image.Options fields plus extension-
threaded params. Stability's edit / upscale / control / video surfaces ship at
sibling paths under /v2beta but require a different request shape; they're not
modeled here. See https://platform.stability.ai/docs/api-reference for the full
reference.

## Install

```bash
go get github.com/Tangerg/scope/models/stability
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
