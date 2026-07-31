// Package mistral wraps Mistral AI's API.
//
// Mistral exposes:
//
//   - /chat/completions — native structured content and thinking chunks, used
//     via [NewChat];
//   - /embeddings — OpenAI-compatible, used via [NewEmbeddingModel]
//     (returns an [openai.EmbeddingModel]);
//   - /moderations — Mistral-native shape that doesn't match OpenAI's
//     moderation response; [NewModerationModel] handles it directly
//     against [API] from this package.
//
// Additional Mistral surfaces not exposed here:
//   - /agents (stateful agent runs);
//   - /fim (code completion endpoint; it is not modeled by the chat facade).
//
// See https://docs.mistral.ai/ for the full API reference.
package mistral
