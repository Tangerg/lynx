// Package nomic wraps Nomic Atlas' embedding API. Nomic publishes
// open-weight, matryoshka-trained, task-conditioned text embedders
// (nomic-embed-text-v1.5 / v1) behind a managed REST surface.
//
// Nomic-specific knobs that don't fit the generic surface — task_type
// (search_query / search_document / classification / clustering for
// asymmetric retrieval and downstream tasks) and long_text_mode —
// are reached via the extension-threaded SDK params, see
// [EmbeddingRequest] and [OptionsKey].
//
// Nomic's /embedding/text request shape uses `texts` (not OpenAI's
// `input`), `task_type` (not `input_type`), and `dimensionality`
// (not `dimensions` / `output_dimension`); this package implements
// [embedding.Model] directly against the native API.
//
// See https://docs.nomic.ai/reference/endpoints/nomic-embed-text
// for the full reference.
package nomic
