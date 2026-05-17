package fireworks

const (
	Provider = "Fireworks"

	BaseURL = "https://api.fireworks.ai/inference/v1"
)

// See https://fireworks.ai/models for the full catalog — Fireworks
// prefixes model ids with "accounts/fireworks/models/".
const (
	ModelLlamaV3p3_70BInstruct  = "accounts/fireworks/models/llama-v3p3-70b-instruct"
	ModelLlamaV3p1_405BInstruct = "accounts/fireworks/models/llama-v3p1-405b-instruct"
	ModelDeepSeekV3             = "accounts/fireworks/models/deepseek-v3"
	ModelDeepSeekR1             = "accounts/fireworks/models/deepseek-r1"
)
