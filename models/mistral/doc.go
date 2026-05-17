// Package mistral wraps Mistral AI's API.
//
// Mistral exposes:
//
//   - /chat/completions — OpenAI-compatible, used via
//     [NewOpenAIChatModel] (returns an [openai.ChatModel]);
//   - /embeddings — OpenAI-compatible, used via [NewEmbeddingModel]
//     (returns an [openai.EmbeddingModel]);
//   - /moderations — Mistral-native shape that doesn't match OpenAI's
//     moderation response; [NewModerationModel] handles it directly
//     against [Api] from this package.
//
// Mistral-specific surfaces not exposed here:
//   - /agents (stateful agent runs);
//   - /fim (code completion endpoint — call from the openai facade
//     using Extra-threaded params).
//
// See https://docs.mistral.ai/ for the full API reference.
package mistral
