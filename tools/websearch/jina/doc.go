// Package jina wires Jina's Search API into [websearch.Provider].
//
// # Endpoint
//
// GET https://s.jina.ai/<url-encoded-query>?<params>
//
// Authentication is a bearer token in the Authorization header.
// Two extra headers shape the response:
//   - Accept: application/json
//   - X-Respond-With: no-content (skip page bodies; we only want
//     the result list for search)
//
// # Parameter mapping
//
// lynx [websearch.Request] → Jina query params:
//   - Query          → URL path segment
//   - MaxResults     → num + count (both required by Jina; clamped
//     to [1, 20])
//   - AllowedDomains → site=comma,separated (Jina's allow-list)
//   - BlockedDomains → not directly supported; ignored
//   - Recency        → noCache=true (forces fresh crawl)
//
// Hardcoded: type=web, provider/engine=google, fallback=true,
// respondWith=markdown, retainImages=none, retainLinks=all, page=1.
//
// # Response mapping
//
// Jina data[] → []*[websearch.Result]:
//   - title       → Title
//   - url         → URL
//   - description → Snippet (falls back to truncated content)
//   - date        → PublishedTime
//
// # Native API
//
// For full parameter access (provider/engine, Bing fallback,
// intitle:/site:/filetype: operators, RetainImages, NoCache,
// Timeout) call [Client.SearchNative] with the provider's own
// [Request] / [Response] types.
//
// # Reference
//
// https://jina.ai/reader#search
package jina
