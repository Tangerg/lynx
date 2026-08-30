// Package cohere implements Core embedding and reranking with Cohere's v2 API.
//
// Embedding callers select the official input_type explicitly because query,
// document, classification, and clustering embeddings have different task
// semantics. Reranking returns indices into the caller-owned document batch.
//
// See https://docs.cohere.com/ for the full API reference.
package cohere
