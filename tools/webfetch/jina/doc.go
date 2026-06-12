// Package jina wires Jina Reader into [webfetch.Provider].
//
// # Endpoint
//
// POST https://r.jina.ai/ (with the target URL in the JSON body).
// Jina also accepts GET https://r.jina.ai/<target> but the POST form
// is more robust for URLs that contain query strings.
//
// Authentication is a bearer token in the Authorization header.
// Format selection happens through headers:
//   - X-Return-Format: markdown | html | text
//   - X-Retain-Images: none (we strip images by default — agents
//     rarely need them and they bloat the LLM context)
//
// # Parameter mapping
//
// [webfetch.Request] → Jina:
//   - URL    → {"url": ...} in body
//   - Format → X-Return-Format header (markdown is the default)
//
// # Response mapping
//
//	data.content → [webfetch.Response.Content]
//
// title, description, and url fields are present in the response
// but not surfaced — the LLM gets the page body, not the metadata.
//
// # Native API
//
// For full parameter access (X-Respond-With for ReaderLM-v2,
// X-Instruction for natural-language extraction, RetainImages /
// RetainLinks modes, WithGeneratedAlt) call [Client.FetchNative]
// with the provider's own [Request] / [Response] types.
//
// # Reference
//
// https://jina.ai/reader
package jina
