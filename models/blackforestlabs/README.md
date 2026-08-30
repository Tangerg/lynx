# blackforestlabs

Package blackforestlabs wraps Black Forest Labs' FLUX image generation API.
NewImageModel targets the official asynchronous /v1/{model} endpoints, follows
each response's polling_url, and downloads the short-lived signed output before
returning it. FLUX-specific knobs (steps, guidance, raw, safety_tolerance,
output_format, prompt_upsampling, image_prompt for img2img / kontext editing)
ride through extension-threaded params. See https://docs.bfl.ai/ for the full
reference.

## Install

```bash
go get github.com/Tangerg/scope/models/blackforestlabs
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
