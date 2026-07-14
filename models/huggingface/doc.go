// Package huggingface exposes the HuggingFace Inference Router, which
// is OpenAI-compatible — chat completions hit /v1/chat/completions with
// the same request/response shape.
//
// Rather than maintain a parallel chat implementation,
// [NewOpenAIChat] returns an [openai.Chat] pre-configured with
// the HuggingFace router base URL. Callers get every feature of the
// openai provider (tool calling, streaming, response accumulation,
// etc.) for free, and the lynx provider matrix records HuggingFace as
// a first-class entry.
package huggingface
