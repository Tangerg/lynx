// Package google wraps Google's Generative Language / Gemini APIs.
//
// Constructors:
//
//   - [NewChatModel] — native genai chat. Full Gemini surface:
//     thinking budget, response modalities, system instructions,
//     safety settings, structured output, tool calling, grounding
//     with Google Search;
//   - [NewOpenAIChatModel] — Gemini's first-party OpenAI-compat
//     bridge at [BaseURLOpenAI] (generativelanguage.googleapis.com/
//     v1beta/openai). Wire-format-only conversion;
//   - [NewEmbeddingModel] — text-embedding-004 / gemini-embedding-001
//     with output_dimensionality truncation;
//   - [NewImageModel] — Imagen 4 / 3 / 2 image generation;
//   - [NewAudioTTSModel] — Gemini-TTS via generate_content with
//     audio response modality;
//   - [NewAudioTranscriptionModel] — audio-input → text via
//     generate_content (Gemini transcribes any audio attachment).
//
// Token estimation: [NewTextEstimator] wraps CountTokens for
// model-specific tokenizer-based counts.
//
// Gemini's Context Caching API (cheaper repeated prompts) doesn't fit
// core/model's interfaces and is not exposed.
//
// genai supports two backends: Generative Language (api key) and
// Vertex AI (project / location). Select via ChatModelConfig.Backend.
//
// See https://ai.google.dev/ for the full reference.
package google
