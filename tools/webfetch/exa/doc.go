// Package exa wires Exa Contents into [webfetch.Provider].
//
// # Endpoint
//
// POST https://api.exa.ai/contents
//
// Authentication uses the x-api-key header.
//
// # Parameter mapping
//
// lynx [webfetch.Request] → Exa request:
//   - URL    → urls=[<single>] (Exa accepts batches; we send one)
//   - Format → text.includeHtmlTags: true when FormatHTML is asked,
//     false otherwise. Exa returns the page as a single `text`
//     field — HTML markup is inlined into it when includeHtmlTags is
//     true. Text and markdown are equivalent on this provider.
//
// # Response mapping
//
//	results[0].text → [webfetch.Response.Content]
//
// title, summary, highlights, and other Exa-specific fields are not
// surfaced — the LLM gets the page body only. To get summaries you'd
// use the search tool with contents.summary enabled.
//
// # Native API
//
// For full parameter access (Highlights with custom query,
// Summary.Schema for structured output, Subpages crawling, IDs as
// alternative to URLs, MaxAgeHours cache control, Extras with
// links / imageLinks) call [Client.FetchNative] with the provider's
// own [Request] / [Response] types.
//
// # Reference
//
// https://exa.ai/docs/reference/get-contents
package exa
