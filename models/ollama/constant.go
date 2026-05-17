package ollama

const (
	Provider = "Ollama"
)

const (
	OptionsKey = "lynx:ai:model:ollama_options"

	// DefaultBaseURL is Ollama's default local listen address.
	DefaultBaseURL = "http://127.0.0.1:11434"

	// OpenAICompatPath is the suffix Ollama serves the OpenAI-compatible
	// API under. [NewOpenAIChatModel] joins it with the configured host.
	OpenAICompatPath = "/v1"
)
