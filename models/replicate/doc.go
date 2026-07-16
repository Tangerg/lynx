// Package replicate wraps Replicate's image and TTS surfaces.
//
// Replicate is a single-call prediction-job platform: every model has
// its own input schema, and a generation runs asynchronously via
// submit → poll → fetch. This package implements [image.Model] and
// [tts.Model] for that flow — same shape as [models/midjourney/luma/
// blackforestlabs/prodia] (image) and [models/elevenlabs/hume/lmnt] (tts).
//
// Modalities exposed:
//
//   - [NewImageModel] — FLUX (schnell / dev / pro / 1.1-pro /
//     1.1-pro-ultra / kontext), SDXL, SD 3.5, Ideogram, and the
//     long tail of community fine-tunes;
//   - [NewAudioTTSModel] — open-weight TTS that doesn't ship as a
//     commercial API: XTTS-v2 (voice cloning, 17 languages), Bark
//     (speech + song + sfx), Tortoise-TTS (highest quality, slow).
//
// Why not chat. Replicate's chat models need model-specific chat
// templates that lynx doesn't want to embed — Llama-3 / Mistral /
// Qwen all expect different formatting. The same upstream models
// route through OpenRouter / Together / Groq / SambaNova / Fireworks
// (all OpenAI-compatible) with proper tool calling and message
// history. Use those.
//
// Why not transcription. Replicate's STT is mostly Whisper variants —
// Deepgram / AssemblyAI / Gladia / RevAI provide stronger SLAs,
// diarization quality, and feature coverage. lynx already supports
// all four.
//
// Model id format. [image.Options].Model selects the Replicate
// model. Two shapes are accepted:
//
//   - "owner/name" — official models pinned to the latest version,
//     e.g. "black-forest-labs/flux-schnell". Posts to
//     /v1/models/{owner}/{name}/predictions.
//   - "owner/name:version_hash" — community models or version-pinned
//     official models, e.g.
//     "stability-ai/sdxl:39ed52f2". Posts to /v1/predictions with
//     the hash in the request body.
//
// Provider-specific input fields. lynx maps [image.Options]
// Width / Height / Seed / NegativePrompt / OutputFormat onto the
// names Replicate's most popular image models share (width / height /
// seed / negative_prompt / output_format). Anything model-specific
// (num_outputs, guidance_scale, num_inference_steps, aspect_ratio,
// scheduler, ...) rides through an [ReplicateInput] / extension-threaded
// PredictionRequest passed via [options.GetParams] under
// [OptionsKey]; lynx forwards your input dict verbatim.
//
// See https://replicate.com/docs/reference/http for the full API.
package replicate
