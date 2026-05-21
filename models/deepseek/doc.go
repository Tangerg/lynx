// Package deepseek wraps DeepSeek's OpenAI-compatible API.
//
// DeepSeek's wire format follows OpenAI's chat-completions spec
// exactly; [NewOpenAIChatModel] therefore returns a pre-configured
// [openai.ChatModel] rather than re-implementing the SDK.
//
// Provider-specific features the openai facade already supports
// transparently:
//
//   - reasoning_content on assistant messages from deepseek-reasoner
//     (lynx [openai.ChatModel] reads it from JSON.ExtraFields and
//     emits a [chat.ReasoningPart] in AssistantMessage.Parts automatically).
//
// Provider-specific features that need explicit BaseURL switching:
//
//   - prefix completion (assistant messages with "prefix": true) must
//     be sent to BaseURLBeta — set [OpenAIChatModelConfig.BaseURL] to
//     [BaseURLBeta] when using this mode. The "prefix" / "fim_*"
//     fields ride through Extra-threaded openai params.
//
// See https://api-docs.deepseek.com/ for the full API reference.
package deepseek
