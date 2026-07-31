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
// Provider-specific request controls use the typed [RequestOptions] extension;
// the OpenAI SDK request shape is intentionally not exposed. Prefix completion
// remains a separate beta protocol and is not accepted by [OpenAIChat].
//
// See https://api-docs.deepseek.com/ for the full API reference.
package deepseek
