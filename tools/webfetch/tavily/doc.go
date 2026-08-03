// Package tavily wires Tavily Extract into [webfetch.Provider].
//
// # Endpoint
//
// POST https://api.tavily.com/extract
//
// Authentication is a bearer token in the Authorization header.
//
// # Parameter mapping
//
// [webfetch.Request] → Tavily request:
//   - URL    → urls=[<single>] (Tavily accepts batches; a single URL is sent)
//   - Format → format. Tavily's enum only supports "markdown" and
//     "text". When the caller asks for HTML, the provider silently maps to
//     markdown and report the effective format in the response.
//
// Hardcoded: extract_depth="basic" (1 credit per 5 URLs; "advanced"
// is 2× more and captures tables / embedded content but adds
// latency).
//
// # Response mapping
//
//	results[0].raw_content → [webfetch.Response.Content]
//
// When the requested URL fails, failed_results[] is populated and the
// error message is surfaced.
//
// Tavily's batched transport DTOs stay private to the provider package.
//
// # Reference
//
// https://docs.tavily.com/documentation/api-reference/endpoint/extract
package tavily
