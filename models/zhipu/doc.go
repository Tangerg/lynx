// Package zhipu wraps Zhipu AI's chat APIs (GLM family).
//
// BigModel serves the GLM chat surface in two compatibility flavors:
//
//   - OpenAI-compatible at /api/paas/v4 — use [NewOpenAIChat];
//   - Anthropic-compatible at /api/anthropic — use
//     [NewAnthropicChat], available for GLM-4.5 and GLM-4.6.
//     Swap base URL and keep
//     their existing integration.
//
// Embedding (embedding-3 / embedding-2) only has the OpenAI flavor
// and goes through [NewEmbeddingModel].
//
// Zhipu-specific surfaces (CogView image generation, CogVideoX video)
// sit on separate endpoints and aren't exposed by this package.
//
// See https://docs.bigmodel.cn/ for the API reference.
package zhipu
