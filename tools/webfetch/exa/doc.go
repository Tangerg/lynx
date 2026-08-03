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
// [webfetch.Request] → Exa request:
//   - URL    → urls=[<single>] (Exa accepts batches; a single URL is sent)
//   - Format → text.includeHtmlTags: true when FormatHTML is asked,
//     false otherwise. Exa returns the page as a single `text`
//     field — HTML markup is inlined into it when includeHtmlTags is
//     true. Text and markdown are equivalent on this provider.
//
// # Response mapping
//
//	results[0].text → [webfetch.Response.Content]
//
// Exa-specific response fields and transport DTOs stay private; the caller
// receives only the requested page body.
//
// # Reference
//
// https://exa.ai/docs/reference/get-contents
package exa
