package huggingface

const (
	Provider = "HuggingFace"
)

const (
	OptionsKey = "huggingface/options"

	// DefaultBaseURL targets the HuggingFace router which proxies to a
	// curated set of inference providers (together, fireworks, nebius,
	// sambanova, hf-inference, ...). The router exposes an
	// OpenAI-compatible /chat/completions endpoint.
	DefaultBaseURL = "https://router.huggingface.co/v1"
)
