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
//   - URL    → urls=[<single>] (Tavily accepts batches; we send one)
//   - Format → format. Tavily's enum only supports "markdown" and
//     "text". When the caller asks for HTML we silently map to
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
// When the requested URL fails, failed_results[] is populated and we
// surface the error message.
//
// # Native API
//
// For full parameter access (advanced extract_depth, query-based
// reranking, IncludeImages, IncludeFavicon, batched URLs, Timeout,
// IncludeUsage) call [Client.FetchNative] with the provider's own
// [Request] / [Response] types.
//
// # Reference
//
// https://docs.tavily.com/documentation/api-reference/endpoint/extract
package tavily
