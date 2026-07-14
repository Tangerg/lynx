// Package deepseek wraps DeepSeek's OpenAI-compatible API.
//
// DeepSeek's wire format follows OpenAI's chat-completions spec
// exactly; [NewOpenAIChat] therefore returns a pre-configured
// [openai.Chat] rather than re-implementing the mapping.
//
// Provider-specific features the openai facade already supports
// transparently:
//
//   - reasoning_content on assistant messages from deepseek-reasoner
//     ([openai.Chat] reads it from the provider response and
//     emits a [chat.ReasoningPart] in AssistantMessage.Parts automatically).
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
