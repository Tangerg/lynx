// Package huggingface exposes the HuggingFace Inference Router, which
// is OpenAI-compatible — chat completions hit /v1/chat/completions with
// the same request/response shape.
//
// [NewChat] returns the provider-local [Chat], backed by the shared
// OpenAI Chat Completions protocol and configured for the Hugging Face router.
// Callers receive tool calling and streaming without depending on OpenAI's
// concrete adapter type.
package huggingface
