// Package tavily wires Tavily's Search API into [websearch.Provider].
//
// # Endpoint
//
// POST https://api.tavily.com/search
//
// Authentication is a bearer token in the Authorization header
// (resty's SetAuthToken).
//
// # Parameter mapping
//
// [websearch.Request] → Tavily request:
//   - Query          → query (required)
//   - MaxResults     → max_results (clamped to [1, 20]; default 5 when 0)
//   - AllowedDomains → include_domains (Tavily caps at 300)
//   - BlockedDomains → exclude_domains (Tavily caps at 150)
//   - Recency        → time_range: hour/day → "day", week→"week",
//     month→"month", year→"year". Tavily's minimum granularity is
//     "day" so RecencyHour is effectively the same as RecencyDay.
//
// Hardcoded knobs: search_depth="basic" (cheapest), topic="general",
// include_favicon=true.
//
// # Response mapping
//
// Tavily result → [websearch.Result]:
//   - title   → Title
//   - url     → URL
//   - content → Snippet
//   - favicon → FaviconURL
//
// The score, answer, images, and request_id fields are ignored.
//
// # Native API
//
// For full parameter access (Answer, Images, Usage, ChunksPerSource,
// IncludeAnswer, etc.) call [Client.SearchNative] with the
// provider's own [Request] / [Response] types instead of the
// SPI's slimmer [websearch.Request] / [websearch.Response].
//
// # Reference
//
// https://docs.tavily.com/documentation/api-reference/endpoint/search
package tavily
