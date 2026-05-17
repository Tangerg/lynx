// Package voyage wraps Voyage AI's embedding API.
//
// Voyage publishes retrieval-tuned text and multimodal embedding
// models that consistently lead public retrieval benchmarks; the
// flagship voyage-3-large / voyage-3 / voyage-3-lite models support
// matryoshka-style output truncation via the output_dimension
// parameter.
//
// Voyage's /embeddings shape is bespoke (input_type, truncation,
// quantization knobs) and doesn't speak the OpenAI dialect — this
// package implements [embedding.Model] directly against the native
// API.
//
// See https://docs.voyageai.com/ for the full reference.
package voyage
