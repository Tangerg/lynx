// Package cohere wraps Cohere's v2 embedding API.
//
// Only the /embed surface is exposed here. Cohere's chat (with
// documents / citations / web-search / connectors) doesn't map
// cleanly onto chat.Model interface and its chat lineage
// trails the OpenAI / Anthropic / Google frontier — embedding is
// where Cohere still leads, especially for multilingual retrieval
// (embed-multilingual-v3.0) and image embedding (embed-v4.0).
//
// See https://docs.cohere.com/ for the full API reference.
package cohere
