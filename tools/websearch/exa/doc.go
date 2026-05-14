// Package exa wires Exa's Search API into [websearch.Provider].
//
// # Endpoint
//
// POST https://api.exa.ai/search
//
// Authentication uses the x-api-key header. (Exa also accepts
// Authorization: Bearer; we use the x-api-key form for clarity.)
//
// # Parameter mapping
//
// lynx [websearch.Request] → Exa request:
//   - Query          → query (required)
//   - MaxResults     → numResults (clamped to [1, 100])
//   - AllowedDomains → includeDomains (Exa caps at 1200)
//   - BlockedDomains → excludeDomains (same cap)
//   - Recency        → startPublishedDate (RFC3339, computed from
//     time.Now() − the recency window)
//
// Hardcoded: type="fast" (cheaper than the default "auto"),
// contents.summary.query=<query> (asks Exa to summarise each result
// against the query). Highlights and full text are not requested.
//
// Not used: category (lynx's SearchType was removed from the SPI for
// being too provider-specific), userLocation, moderation.
//
// # Response mapping
//
// Exa results[] → []*[websearch.Result]:
//   - title         → Title
//   - url           → URL
//   - publishedDate → PublishedTime (RFC3339)
//   - author        → Source
//   - favicon       → FaviconURL
//   - highlights[0] → Snippet (falls back to summary)
//
// Exa does not echo the original query; the tool forwards what the
// caller supplied.
//
// # Native API
//
// For full parameter access (Category, Contents.Text/Highlights/
// Summary configuration, IncludeText/ExcludeText, AdditionalQueries
// for deep search, CostDollars in the response) call
// [Client.SearchNative] with the provider's own [Request] /
// [Response] types.
//
// # Reference
//
// https://exa.ai/docs/reference/search
package exa
