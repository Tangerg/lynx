// Package voyage implements Core embedding and reranking with Voyage AI.
//
// Voyage publishes retrieval-tuned text and multimodal embedding
// models that consistently lead public retrieval benchmarks; the
// current voyage-4-large / voyage-4 / voyage-4-lite models support
// matryoshka-style output truncation via the output_dimension
// parameter.
//
// Voyage's /embeddings shape is bespoke (input_type, truncation,
// quantization knobs) and doesn't speak the OpenAI dialect — this
// package implements [embedding.Model] directly against the native
// API. Reranking keeps truncation provider-specific while returning Core
// document indices and normalized relevance scores.
//
// See https://docs.voyageai.com/ for the full reference.
package voyage
