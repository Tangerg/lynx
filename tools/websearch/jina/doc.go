// Package jina wires Jina's Search API into [websearch.Provider].
//
// # Endpoint
//
// GET https://s.jina.ai/<url-encoded-query>?<params>
//
// Authentication is a bearer token in the Authorization header.
// Two extra headers shape the response:
//   - Accept: application/json
//   - X-Respond-With: no-content (skip page bodies; only the
//     result list is needed for search)
//
// # Parameter mapping
//
// [websearch.Request] → Jina query params:
//   - Query          → URL path segment
//   - MaxResults     → count (clamped to [1, 20])
//   - AllowedDomains → site=comma,separated (Jina's allow-list)
//   - BlockedDomains → not directly supported; ignored
//   - Recency        → noCache=true (forces fresh crawl)
//
// Hardcoded: X-Respond-With=no-content and page=1.
//
// # Response mapping
//
// Jina data[] → []*[websearch.Result]:
//   - title       → Title
//   - url         → URL
//   - description → Snippet (falls back to truncated content)
//   - date        → PublishedTime
//
// Jina's query and response DTOs remain private to the provider package.
//
// # Reference
//
// https://jina.ai/reader#search
package jina
