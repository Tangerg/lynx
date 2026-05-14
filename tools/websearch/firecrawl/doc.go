// Package firecrawl wires Firecrawl's Search API into [websearch.Provider].
//
// Firecrawl is primarily a scraping service — see
// [github.com/Tangerg/lynx/tools/webfetch/firecrawl] for the /scrape
// counterpart that uses the same API key.
//
// # Endpoint
//
// POST https://api.firecrawl.dev/v2/search
//
// Authentication is a bearer token in the Authorization header.
//
// # Parameter mapping
//
// lynx [websearch.Request] → Firecrawl request:
//   - Query          → query (after Google-style site:/-site:
//     rewriting; Firecrawl has no native allow/block fields)
//   - MaxResults     → limit (clamped to [1, 100]; default 10)
//   - AllowedDomains → inlined as `site:foo.com` operators
//   - BlockedDomains → inlined as `-site:foo.com` operators
//   - Recency        → tbs=qdr:h|d|w|m|y
//
// Only the "web" vertical is wired through the SPI. Use
// [Client.SearchNative] to access news / images via Sources.
//
// # Response mapping
//
// Firecrawl data.web[] → []*[websearch.Result]:
//   - title       → Title
//   - url         → URL
//   - description → Snippet
//
// # Native API
//
// For full parameter access (Sources for news / images verticals,
// Location for fine-grained geo, ScrapeOptions to inline rendered
// content into every result) call [Client.SearchNative] with the
// provider's own [Request] / [Response] types. ScrapeOptions is the
// killer feature: one round-trip gets you ranked SERP results AND
// their fully rendered markdown.
//
// # Reference
//
// https://docs.firecrawl.dev/api-reference/endpoint/search
package firecrawl
