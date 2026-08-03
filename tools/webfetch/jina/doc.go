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
//   - X-Retain-Images: none (images are stripped by default — agents
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
// Jina-specific response metadata and transport DTOs stay private; the
// caller receives only the page body.
//
// # Reference
//
// https://jina.ai/reader
package jina
