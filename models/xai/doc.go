// Package xai wraps xAI's (Grok) OpenAI-compatible API.
//
// [NewOpenAIChat] returns xAI's provider-local [OpenAIChat], backed by the
// shared OpenAI Chat Completions protocol.
//
// Current Grok models support text, image input, structured outputs, reasoning
// effort, and custom function calling through the Chat Completions surface.
// xAI's server-side Web Search, X Search, code execution, and collections tools
// belong to its Responses API surface and are not represented as Core custom
// functions by this adapter.
//
// See https://docs.x.ai/ for the full API reference.
package xai
