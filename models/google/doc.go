// Package google wraps Google's Generative Language / Gemini APIs.
//
// Constructors:
//
//   - [NewChat] — native genai chat. Full Gemini surface:
//     thinking budget, response modalities, system instructions,
//     safety settings, structured output, tool calling, grounding
//     with Google Search;
//   - [NewOpenAIChat] — Gemini's first-party OpenAI-compat
//     bridge at [BaseURLOpenAI] (generativelanguage.googleapis.com/
//     v1beta/openai). Wire-format-only conversion;
//   - [NewEmbeddingModel] — gemini-embedding-2
//     with output_dimensionality truncation;
//   - [NewImageModel] — Gemini image generation through Interactions;
//   - [NewAudioTTSModel] — Gemini-TTS via generate_content with
//     audio response modality;
//   - [NewAudioTranscriptionModel] — audio-input → text via
//     generate_content (Gemini transcribes any audio attachment).
//
// Token estimation: [NewTextEstimator] wraps CountTokens for
// model-specific tokenizer-based counts.
//
// Gemini's Context Caching API (cheaper repeated prompts) doesn't fit
// core/chat's request model and is not exposed.
//
// genai supports two backends: Generative Language (api key) and
// Vertex AI has its own facade package and construction config.
//
// See https://ai.google.dev/ for the full reference.
package google
