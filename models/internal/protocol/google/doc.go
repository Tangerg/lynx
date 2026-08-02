// Package google implements the Google Gen AI wire protocol reused by Gemini
// and Vertex AI provider endpoints inside the models module.
//
// Constructors:
//
//   - [NewChat] — native genai chat. Full Gemini surface:
//     thinking budget, response modalities, system instructions,
//     safety settings, structured output, tool calling, grounding
//     with Google Search;
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
package google
