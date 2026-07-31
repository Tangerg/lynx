// Package deepseek wraps DeepSeek's OpenAI-compatible API.
//
// DeepSeek derives from the OpenAI Chat Completions protocol while retaining
// its provider-specific reasoning semantics behind [OpenAIChat].
//
// Provider-specific behavior handled transparently:
//
//   - reasoning_content is decoded into a [chat.PartReasoning];
//   - ordinary prior assistant reasoning is omitted on later turns;
//   - reasoning associated with tool calls is replayed as reasoning_content.
//
// Provider-specific features that need explicit BaseURL switching:
//
//   - prefix completion (assistant messages with "prefix": true) must
//     be sent to BaseURLBeta — set [OpenAIChatConfig.BaseURL] to
//     [BaseURLBeta] when using this mode. Provider-specific request fields use
//     the namespaced OpenAI request extension.
//
// See https://api-docs.deepseek.com/ for the full API reference.
package deepseek
