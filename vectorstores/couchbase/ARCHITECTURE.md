# couchbase architecture

> One vector-store backend. It owns its config, its wire encoding, and its
> provider identifiers, and nothing else.

The family contract is [`../ARCHITECTURE.md`](../ARCHITECTURE.md); repository-
wide rules are in [`../../AGENTS.md`](../../AGENTS.md). The usage entry point
is [`README.md`](README.md).

---

## 1. What this module owns

- A validated `Config` and the constructors it produces.
- The implementation of the capability interfaces in `core/vectorstore` that
  this service genuinely supports.
- Its own provider identifiers, schema, and wire encoding.

## 2. What it does not own

- Protocol semantics. Those belong to `core`; this module never changes a
  contract to fit one vendor.
- Retries. The underlying SDK already retries.
- OAuth or token refresh. The caller supplies a credential, and a 401 is a
  prompt to re-enter it.
- Observability. An outer decorator in the `otel` module adds spans and
  metrics; nothing here imports OpenTelemetry.
- Test infrastructure. The conformance suite lives with the contract owner
  and is run, never copied.

## 3. Dependency island

This module is an independent release unit. It requires only its own third-
party SDK and the Core packages it implements. It must never import a sibling
provider, and in-repository cooperation happens through `go.work` alone — never
a `replace` directive.

Direct third-party requirements:

- `github.com/couchbase/gocb/v2`
- `github.com/samber/lo`
