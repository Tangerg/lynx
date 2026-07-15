package google

const (
	Provider = "Google"
)

const (
	OptionsKey = "google/options"
)

// BaseURLOpenAI is Gemini's first-party OpenAI-compatible endpoint
// (https://ai.google.dev/gemini-api/docs/openai). Use it via
// [NewOpenAIChat] to reach Gemini models with the OpenAI SDK.
const BaseURLOpenAI = "https://generativelanguage.googleapis.com/v1beta/openai"
