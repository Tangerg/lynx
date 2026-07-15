package replicate

const (
	Provider = "Replicate"
)

const (
	OptionsKey = "replicate/options"

	// DefaultBaseURL is Replicate's production API root.
	DefaultBaseURL = "https://api.replicate.com/v1"

	// DefaultPollIntervalSeconds is how long Call waits between
	// status polls. Image jobs on Replicate take anywhere from 1s
	// (flux-schnell) to 30s (flux-1.1-pro-ultra, SDXL with high
	// step count); 1s is short enough to hide tail latency without
	// hammering the API.
	DefaultPollIntervalSeconds = 1

	// DefaultPollTimeoutSeconds is the wall-clock cap on a single
	// image generation. Bumps above 120s should target slow models
	// like SDXL with high inference steps or large-batch num_outputs.
	DefaultPollTimeoutSeconds = 180

	// DefaultTTSPollTimeoutSeconds is the wall-clock cap on a TTS
	// job. Bark and especially Tortoise-TTS can spend a long time
	// on cold starts plus generation, so the cap is set higher than image.
	DefaultTTSPollTimeoutSeconds = 300
)

// Common image model ids on Replicate. The "owner/name" shape pins
// to whatever the model owner has marked latest — Replicate handles
// the version resolution server-side.
const (
	// ModelFluxSchnell is the cheapest / fastest FLUX (4-step
	// distilled, ~1s).
	ModelFluxSchnell = "black-forest-labs/flux-schnell"

	// ModelFluxDev is the open-weights FLUX dev model.
	ModelFluxDev = "black-forest-labs/flux-dev"

	// ModelFluxPro is the original commercial FLUX (50 steps).
	ModelFluxPro = "black-forest-labs/flux-pro"

	// ModelFlux11Pro (flux-1.1-pro) is the newer pro variant with
	// improved prompt adherence.
	ModelFlux11Pro = "black-forest-labs/flux-1.1-pro"

	// ModelFlux11ProUltra (flux-1.1-pro-ultra) targets up-to-4MP
	// renders.
	ModelFlux11ProUltra = "black-forest-labs/flux-1.1-pro-ultra"

	// ModelFluxKontextPro / Max are FLUX's instruction-driven editing
	// models — pass an input image via Extra "input_image" + a prompt
	// describing the edit.
	ModelFluxKontextPro = "black-forest-labs/flux-kontext-pro"
	ModelFluxKontextMax = "black-forest-labs/flux-kontext-max"

	// ModelSDXL is Stability's SDXL — version-pinned community model;
	// use the ":version" suffix to lock to a tested hash.
	ModelSDXL = "stability-ai/sdxl"

	// ModelSD35Large is Stable Diffusion 3.5 Large.
	ModelSD35Large = "stability-ai/stable-diffusion-3.5-large"

	// ModelIdeogramV2 is Ideogram V2 — strong on typography and
	// poster compositions.
	ModelIdeogramV2 = "ideogram-ai/ideogram-v2"

	// ModelIdeogramV2Turbo is the fast / cheap Ideogram variant.
	ModelIdeogramV2Turbo = "ideogram-ai/ideogram-v2-turbo"
)

// Common TTS model ids on Replicate. These are open-weight models
// not available through commercial TTS APIs — for hosted commercial
// voices use [models/elevenlabs/hume/lmnt] / [models/openai] /
// [models/deepgram] / [models/google] instead.
const (
	// ModelXTTSV2 (Coqui XTTS-v2) is a multilingual open voice-clone
	// model. Supports 17 languages and zero-shot cloning from a 6-
	// second reference WAV passed via Extra "speaker_wav".
	ModelXTTSV2 = "lucataco/xtts-v2"

	// ModelBark (Suno Bark) is an open generative audio model
	// covering speech, song, laughs, and sound effects. Voice
	// presets ride through Extra "history_prompt".
	ModelBark = "suno-ai/bark"

	// ModelTortoiseTTS (mrhan1993/tortoise-tts) is high-quality but
	// slow open TTS. Voice ids go in Extra "voice_a"; quality
	// presets ("ultra_fast", "fast", "standard", "high_quality")
	// via "preset".
	ModelTortoiseTTS = "afiaka87/tortoise-tts"
)
