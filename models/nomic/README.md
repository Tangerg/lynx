# nomic

Package nomic wraps Nomic Atlas' embedding API. Nomic publishes open-weight,
matryoshka-trained, task-conditioned text embedders (nomic-embed-text-v1.5 /
v1) behind a managed REST surface. Nomic-specific knobs that don't fit the
generic surface — task_type (search_query / search_document / classification /
clustering for asymmetric retrieval and downstream tasks) and long_text_mode —
are reached via the extension-threaded SDK params, see [EmbeddingRequest] and
EmbeddingRequestExtensionKey. Nomic's /embedding/text request shape uses
`texts` (not OpenAI's `input`), `task_type` (not `input_type`), and
`dimensionality` (not `dimensions` / `output_dimension`); this package
implements embedding.Model directly against the native API. See
https://docs.nomic.ai/reference/endpoints/nomic-embed-text for the full
reference.

## Install

```bash
go get github.com/Tangerg/scope/models/nomic
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewEmbeddingModel`

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
