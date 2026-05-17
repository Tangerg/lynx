// Package azureaisearch wraps Azure AI Search's vector capabilities
// as a [vectorstore.Store] over the REST API (Azure doesn't ship a
// typed Go SDK for the Search service yet).
//
// Requirements: an Azure AI Search service (Basic tier or higher),
// with an index pre-provisioned through ARM / Terraform / Portal /
// REST. The store does NOT create indexes — Azure AI Search index
// schemas are typed and declared at creation; lynx assumes the
// configured ID / content / vector / metadata fields exist.
//
// Authentication: API key via the `api-key` header. For Managed
// Identity / OAuth, inject a bearer token through a custom
// [http.Client].
//
// Retrieval shape:
//
//	POST /indexes/<index>/docs/search?api-version=2024-07-01
//	{
//	  "size": K, "top": K,
//	  "vectorQueries": [{"kind": "vector", "vector": [...],
//	                     "k": K, "fields": "contentVector"}],
//	  "filter": "<odata>"
//	}
//
// Filter visitor produces OData `$filter` syntax — metadata fields
// must exist as TOP-LEVEL index fields (Azure AI Search doesn't
// support nested-property paths in $filter). LIKE maps to
// `search.ismatch('pattern', 'field')`; IN maps to
// `search.in(field, 'v1,v2,...', ',')`.
//
// Delete uses the `mergeOrUpload` action surface — the store
// enumerates ids that match the filter via paged search, then
// issues a delete batch (1000 ids per request, the service cap).
//
// See https://learn.microsoft.com/azure/search/vector-search-overview.
package azureaisearch
