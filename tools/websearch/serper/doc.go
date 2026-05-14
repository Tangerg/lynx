// Package serper wires Serper's Google Search API into
// [websearch.Provider].
//
// # Endpoint
//
// POST https://google.serper.dev/search
//
// Authentication uses the X-API-KEY header (not bearer).
//
// # Parameter mapping
//
// lynx [websearch.Request] → Serper request:
//   - Query          → q (after Google site:/-site: rewriting via
//     [websearch.BuildSiteOperatorQuery])
//   - MaxResults     → num (forwarded as-is)
//   - AllowedDomains → inlined into q as `site:foo.com site:bar.com`
//   - BlockedDomains → inlined into q as `-site:foo.com -site:bar.com`
//   - Recency        → tbs=qdr:h|d|w|m|y
//
// Serper has no native include/exclude domain fields, so we rewrite
// the query with Google's site:/-site: operators. The trade-off is
// that the operator count counts against Google's per-query token
// budget.
//
// Hardcoded: autocorrect=true.
//
// # Response mapping
//
// Serper organic[] → []*[websearch.Result]:
//   - title   → Title
//   - link    → URL
//   - snippet → Snippet
//   - date    → PublishedTime (best-effort parse; relative strings
//     like "2 days ago" become zero time)
//
// Knowledge graph, answer box, people-also-ask, and related searches
// are ignored — we only surface organic results.
//
// # Native API
//
// For full parameter access (AnswerBox, KnowledgeGraph,
// PeopleAlsoAsk, RelatedSearches, page, location, autocorrect) call
// [Client.SearchNative] with the provider's own [Request] /
// [Response] types instead of the lynx SPI.
//
// # Reference
//
// https://serper.dev/playground
package serper
