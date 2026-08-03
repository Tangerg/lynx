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
// [websearch.Request] → Firecrawl request:
//   - Query          → query (after Google-style site:/-site:
//     rewriting; Firecrawl has no native allow/block fields)
//   - MaxResults     → limit (clamped to [1, 100]; default 10)
//   - AllowedDomains → inlined as `site:foo.com` operators
//   - BlockedDomains → inlined as `-site:foo.com` operators
//   - Recency        → tbs=qdr:h|d|w|m|y
//
// Only the "web" vertical is wired through the provider contract.
//
// # Response mapping
//
// Firecrawl data.web[] → []*[websearch.Result]:
//   - title       → Title
//   - url         → URL
//   - description → Snippet
//
// Firecrawl's transport DTOs remain private so provider-specific response
// shapes cannot leak into callers.
//
// # Reference
//
// https://docs.firecrawl.dev/api-reference/endpoint/search
package firecrawl
