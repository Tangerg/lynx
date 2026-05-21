// Package websearch exposes a single LLM-callable web search tool on
// top of a [Provider] SPI. Each SaaS backend (Tavily, Serper, Jina,
// Exa, Perplexity, ...) lives in its own subpackage and implements
// [Provider]; the tool itself is a thin LLM-facing adapter.
//
// Wire one of the subpackage clients into [NewTool] to get a tool the
// chat layer can invoke. There is no "local" provider — web search
// inherently requires an upstream API.
//
// The argument surface is deliberately small: most LLM-facing knobs
// (country, language, safe-mode, time-range, search type) are too
// provider-specific to be useful at the agent layer. Recency covers
// the common "fresh results" case without per-provider drift.
package websearch
