// Package ollama wraps Ollama's two chat surfaces.
//
// Ollama serves the same models at two different wire formats:
//
//   - Native API at /api/chat — accessed via [NewNativeChatModel]. Gives
//     access to Ollama-specific features (keep_alive, format=json,
//     thinking on supported models, raw "options" dict for fine-grained
//     sampling control).
//   - OpenAI-compatible API at /v1/chat/completions — accessed via
//     [NewOpenAIChatModel]. Works with any OpenAI tooling and benefits
//     from the openai provider's response_format / tool_calling /
//     reasoning_content plumbing.
//
// Pick native when the daemon-specific knobs matter, OpenAI-compat
// when integrating with code already written against the openai API.
//
// Embedding has the same dual surface; lynx ships the native flavor
// as [NewEmbeddingModel]. The OpenAI-compatible /v1/embeddings path
// works through [openai.NewEmbeddingModel] with
// [option.WithBaseURL] pointed at "http://host:11434/v1".
package ollama
