// Package jina wraps Jina AI's embedding API. Jina publishes
// multilingual retrieval embedders (jina-embeddings-v3), multimodal
// CLIP-style models (jina-clip-v2), and ColBERT late-interaction
// models.
//
// Jina-specific knobs that don't fit the generic surface — task type
// ("retrieval.query" / "retrieval.passage" / "text-matching" /
// "classification" / "separation"), late chunking, embedding_type
// (float / int8 / uint8 / binary / ubinary quantization),
// normalization — are reached via the extension-threaded SDK params,
// see [getOptionsParams] and the [EmbeddingRequest] struct.
//
// Jina's /embeddings dialect partially overlaps OpenAI's but uses
// "dimensions" rather than "output_dimension" and exposes the
// task-conditioning field; this package implements [embedding.Model]
// directly against the native API.
//
// See https://jina.ai/embeddings/ for the full reference.
package jina
