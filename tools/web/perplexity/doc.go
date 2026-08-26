// Package perplexity wires Perplexity's Search API into
// [web.Searcher].
//
// # Endpoint
//
// POST https://api.perplexity.ai/search
//
// Authentication is a bearer token in the Authorization header.
//
// # Parameter mapping
//
// [web.SearchRequest] → Perplexity request:
//   - Query          → query (required)
//   - MaxResults     → max_results (omitted when unset)
//   - AllowedDomains → search_domain_filter (capped at 20 entries)
//   - BlockedDomains → search_domain_filter with "-" prefix
//     (capped at 20 entries). The allow- and block-list share the
//     same field; if both are set the caller-validated mutual
//     exclusion makes this a non-issue.
//   - Recency        → search_recency_filter: hour/day/week/month/year
//
// # Response mapping
//
// Perplexity result → [web.SearchResult]:
//   - title   → Title
//   - url     → URL
//   - snippet → Snippet
//   - date    → PublishedTime (parsed as time.DateOnly)
//
// Perplexity does not echo the original query; the tool forwards
// what the caller supplied so [web.SearchResponse.Query] stays meaningful.
//
// Perplexity's transport DTOs stay private to keep the provider boundary
// normalized and self-contained.
//
// # Reference
//
// https://docs.perplexity.ai/api-reference/search-post
package perplexity
