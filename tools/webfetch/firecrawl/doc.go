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
// lynx [webfetch.Request] → Firecrawl request:
//   - URL    → url
//   - Format → formats=[{"type": <format>}]. We always request a
//     single format. Firecrawl supports markdown/html/text plus
//     richer modes (summary, screenshot, links, json) that lynx
//     doesn't expose.
//
// Hardcoded: onlyMainContent=true (strips nav/footer/boilerplate).
//
// # Response mapping
//
// Firecrawl returns multiple format fields simultaneously
// (data.markdown, data.html, data.rawHtml, ...). We pick the field
// matching the caller's requested format:
//
//   - FormatHTML     → data.html (cleaned HTML)
//   - FormatText     → data.rawHtml (full unprocessed HTML)
//   - FormatMarkdown → data.markdown (default)
//
// A success=false top-level field surfaces as a Go error rather than
// being returned as content.
//
// # Native API
//
// For full parameter access (Actions for browser automation —
// Click / Wait / Screenshot / ExecuteJavascript / PDF / etc., custom
// Headers, Location emulation, Proxy tier, IncludeTags / ExcludeTags,
// MaxAge cache control) call [Client.FetchNative] with the provider's
// own [Request] / [Response] types.
//
// # Reference
//
// https://docs.firecrawl.dev/api-reference/endpoint/scrape
package firecrawl
