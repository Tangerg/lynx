package fireworks

const (
	Provider = "Fireworks"

	BaseURL = "https://api.fireworks.ai/inference/v1"
)

// Current serverless model ids. See https://fireworks.ai/models for the live
// catalog — Fireworks prefixes model ids with "accounts/fireworks/models/".
const (
	ModelGPTOSS20B = "accounts/fireworks/models/gpt-oss-20b"
	ModelKimiK26   = "accounts/fireworks/models/kimi-k2p6"
	ModelGLM52     = "accounts/fireworks/models/glm-5p2"
)
