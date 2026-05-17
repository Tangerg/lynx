// Package openai wraps OpenAI's API surface as the canonical
// implementation of lynx's core/model interfaces.
//
// Modalities exposed:
//
//   - chat (Chat Completions) via [NewChatModel] — tool calling,
//     streaming, response_format, reasoning_content auto-routing
//     (used transparently by DeepSeek-R1, Kimi-thinking, QwQ, etc.),
//     vision input, audio input/output;
//   - embedding via [NewEmbeddingModel] — text-embedding-3-small/large
//     with dimension truncation;
//   - image via [NewImageModel] — DALL·E 3 and gpt-image-1;
//   - moderation via [NewModerationModel] — omni-moderation-latest;
//   - audio tts via [NewAudioTTSModel] — tts-1, tts-1-hd, gpt-4o-mini-tts;
//   - audio transcription via [NewAudioTranscriptionModel] —
//     whisper-1, gpt-4o-transcribe, gpt-4o-mini-transcribe;
//   - audio translation via [NewAudioTranslationModel] — whisper-1
//     translating any source language to English (implements
//     transcription.Model).
//
// Model id constants aren't exported here — they're maintained by
// openai-go ([openai.ChatModelGPT4o], [openai.EmbeddingModelTextEmbedding3Large],
// etc.). Import openai-go directly when you need them.
//
// Provider-specific OpenAI fields not modeled by core/model
// (response_format, tools, audio, modalities, vision parts) reach
// the wire via Extra-threaded openai-go params.
//
// See https://platform.openai.com/docs for the full API reference.
package openai
