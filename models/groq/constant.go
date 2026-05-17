package groq

const (
	Provider = "Groq"

	BaseURL = "https://api.groq.com/openai/v1"
)

// See https://console.groq.com/docs/models for the current catalog —
// Groq routinely rotates preview models and deprecates older snapshots.
const (
	ModelLlama33_70BVersatile = "llama-3.3-70b-versatile"
	ModelLlama31_8BInstant    = "llama-3.1-8b-instant"
	ModelGemma2_9BIT          = "gemma2-9b-it"
	ModelDeepSeekR1Distill70B = "deepseek-r1-distill-llama-70b"
	ModelKimiK2               = "moonshotai/kimi-k2-instruct"
)
