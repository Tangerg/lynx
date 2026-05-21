// Package webfetch exposes a single LLM-callable web fetch / scrape
// tool on top of a [Provider] SPI. Each SaaS backend (Jina Reader,
// Firecrawl, Tavily Extract, Exa Contents, ...) lives in its own
// subpackage and implements [Provider]; the tool itself is a thin
// LLM-facing adapter.
//
// Wire one of the subpackage clients into [NewTool] to get a tool the
// chat layer can invoke. There is no "local" provider — fetching
// modern JS-heavy pages is what these SaaS services exist for.
package webfetch
