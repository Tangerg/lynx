# azureaisearch

Package azureaisearch exposes Azure AI Search's vector capabilities through the
Core vector-store capability interfaces over the REST API (Azure doesn't ship a
typed Go SDK for the Search service yet). Requirements: an Azure AI Search
service (Basic tier or higher), with an index pre-provisioned through ARM /
Terraform / Portal / REST. The store does NOT create indexes — Azure AI Search
index schemas are typed and declared at creation; scope assumes the configured
ID / content / vector / metadata fields exist. Authentication: API key via the
`api-key` header. For Managed Identity / OAuth, inject a bearer token through a
custom http.Client. Semantic search sends one vector query. Hybrid search sends
the same vector query together with `search` and restricts lexical evidence to
the configured content field; Azure performs its native result fusion.
Filter visitor produces OData `$filter` syntax — metadata fields must exist as
TOP-LEVEL index fields (Azure AI Search doesn't support nested-property paths
in $filter). LIKE maps to `search.ismatch('pattern', 'field')`; IN maps to
`search.in(field, 'v1,v2,...', ',')`. Delete uses the `mergeOrUpload` action
surface — the store enumerates ids that match the filter via paged search, then
issues a delete batch (1000 ids per request, the service cap). See
https://learn.microsoft.com/azure/search/vector-search-overview.

## Install

```bash
go get github.com/Tangerg/scope/vectorstores/azureaisearch
```

## Constructors

Every constructor validates its config and returns a value implementing
the capability interfaces in `core/vectorstore`:

- `NewStore`

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
