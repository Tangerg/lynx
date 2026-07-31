// Package cohere wraps Cohere's v2 embedding API.
//
// Only the /v2/embed surface is exposed. Callers select the official
// input_type explicitly because query, document, classification, and
// clustering embeddings have different task semantics.
//
// See https://docs.cohere.com/ for the full API reference.
package cohere
