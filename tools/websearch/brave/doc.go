// Package brave wires Brave's Web Search API into [websearch.Provider].
//
// # Endpoint
//
// GET https://api.search.brave.com/res/v1/web/search
//
// Authentication uses the X-Subscription-Token header (not bearer).
//
// # Parameter mapping
//
// lynx [websearch.Request] → Brave query parameters:
//   - Query          → q (after Google-style site:/-site: rewriting)
//   - MaxResults     → count (clamped to [1, 20]; default 10)
//   - AllowedDomains → inlined as `site:foo.com` operators
//   - BlockedDomains → inlined as `-site:foo.com` operators
//   - Recency        → freshness=pd/pw/pm/py (hour collapses to "pd"
//     since Brave's minimum granularity is "past day")
//
// Brave has no native allow/block-domain fields, so we rewrite the
// query with site:/-site: like the Serper provider does.
//
// # Response mapping
//
// Brave web.results[] → []*[websearch.Result]:
//   - title       → Title
//   - url         → URL
//   - description → Snippet
//   - page_age    → PublishedTime (RFC3339; the human-readable
//     "age" field is dropped because relative strings like "2 hours
//     ago" don't parse)
//
// The news/videos/places verticals, the AI summarizer, and rich
// results are not surfaced — call [Client.SearchNative] to access
// the full response.
//
// # Native API
//
// For full parameter access (Safesearch, ResultFilter, GogglesID for
// custom re-ranking, ExtraSnippets, Summary for AI summarisation,
// Country / SearchLang / UILang) call [Client.SearchNative] with
// the provider's own [Request] / [Response] types.
//
// # Why Brave
//
// Brave runs its own independent index (not a Google/Bing reseller),
// so adding it gives the agent a result source that doesn't drift
// with the Google duopoly. Pricing is friendly: ~2000 queries/month
// free, sub-cent per query on paid tiers.
//
// # Reference
//
// https://api-dashboard.search.brave.com/app/documentation/web-search/get-started
package brave
