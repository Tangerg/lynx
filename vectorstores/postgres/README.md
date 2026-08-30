# postgres

`postgres` adapts this backend to the capability interfaces in
`core/vectorstore`.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/postgres
```

## Testing

This module integrates a third-party service, so its tests cover what runs
without live credentials: config validation, request and response mapping, and
error classification. The shared conformance contract is
`core/vectorstore/storetest` — this module runs it rather than copying it.

An integration probe skips unless its credential environment variable is set,
so `go test ./...` is always runnable offline.

## Boundaries

This is an independent leaf module: it carries only its own SDK dependency and
never imports a sibling provider. The shared contract every module in this
family obeys is in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
