// Package firecrawl wires Firecrawl Scrape into [webfetch.Provider].
//
// # Endpoint
//
// POST https://api.firecrawl.dev/v2/scrape
//
// Authentication is a bearer token in the Authorization header.
//
// # Parameter mapping
//
// [webfetch.Request] → Firecrawl request:
//   - URL    → url
//   - Format → formats=[{"type": <format>}]. Markdown and HTML are
//     supported; plain text maps to markdown, the nearest clean format.
//
// Hardcoded: onlyMainContent=true (strips nav/footer/boilerplate).
//
// # Response mapping
//
// FormatHTML maps to data.html; markdown and mapped text requests use
// data.markdown.
//
// A success=false top-level field surfaces as a Go error rather than
// being returned as content.
//
// Firecrawl's transport and browser-automation DTOs stay private to the
// provider boundary.
//
// # Reference
//
// https://docs.firecrawl.dev/api-reference/endpoint/scrape
package firecrawl
