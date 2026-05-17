// Package xai wraps xAI's (Grok) OpenAI-compatible API.
//
// xAI's wire format follows OpenAI's chat-completions spec exactly;
// [NewOpenAIChatModel] returns a pre-configured [openai.ChatModel].
//
// Provider-specific features reachable via Extra-threaded openai
// params:
//
//   - Live-search: pass a `search_parameters` object to enable
//     real-time web / X / news / RSS retrieval. See
//     https://docs.x.ai/docs/guides/live-search.
//   - Vision: Grok 4 and Grok 2 Vision accept image inputs through
//     the standard openai content-part shape.
//
// See https://docs.x.ai/ for the full API reference.
package xai
