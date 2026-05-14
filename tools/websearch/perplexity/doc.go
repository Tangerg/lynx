// Package perplexity wires Perplexity's Search API into
// [websearch.Provider].
//
// # Endpoint
//
// POST https://api.perplexity.ai/search
//
// Authentication is a bearer token in the Authorization header.
//
// # Parameter mapping
//
// lynx [websearch.Request] → Perplexity request:
//   - Query          → query (required)
//   - MaxResults     → max_results (clamped to [1, 20])
//   - AllowedDomains → search_domain_filter (capped at 20 entries)
//   - BlockedDomains → search_domain_filter with "-" prefix
//     (capped at 20 entries). The allow- and block-list share the
//     same field; if both are set the caller-validated mutual
//     exclusion makes this a non-issue.
//   - Recency        → search_recency_filter: hour/day/week/month/year
//
// Not used: max_tokens, max_tokens_per_page, country,
// search_language_filter, search_mode (these are not exposed at the
// lynx SPI layer because they're inconsistent across providers).
//
// # Response mapping
//
// Perplexity result → [websearch.Result]:
//   - title   → Title
//   - url     → URL
//   - snippet → Snippet
//   - date    → PublishedTime (parsed as time.DateOnly)
//
// Perplexity does not echo the original query; the tool forwards
// what the caller supplied so [Response.Query] stays meaningful.
//
// # Native API
//
// For full parameter access (MaxTokens, SearchLanguageFilter,
// Country, exact date filters, LastUpdated filters) call
// [Client.SearchNative] with the provider's own [Request] /
// [Response] types.
//
// # Reference
//
// https://docs.perplexity.ai/api-reference/search-post
package perplexity
