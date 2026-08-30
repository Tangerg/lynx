# perplexity

Package perplexity wraps Perplexity's OpenAI-compatible Sonar API. Every Sonar
model runs an online retrieval step before answering; the response includes
citations + search results. Perplexity-specific knobs are exposed as
RequestOptions through RequestExtensionKey: - "search_mode" ("web" /
"academic"), "search_domain_filter", "search_recency_filter" steer the
underlying web search. - "return_images" / "return_related_questions" toggle
extra response fields. - "web_search_options" controls per-call search behavior
(search context size, user location, etc.). Response extras (citations,
search_results, images, related_questions, detailed usage, and costs) are
preserved from the exact provider JSON in the namespaced Core response
extension. See https://docs.perplexity.ai/ for the full API reference.

## Install

```bash
go get github.com/Tangerg/scope/models/perplexity
```

## Constructors

Every constructor validates its config and returns a value implementing
the `Model` contracts in `core`:

- `NewOpenAIChat`

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
