package azureopenai

const (
	Provider = "AzureOpenAI"
)

const (
	// DefaultAPIVersion is the latest stable Azure OpenAI REST
	// version supported as of 2026-Q2. Pin to a newer string when
	// you need preview-only features (vision tools, structured
	// outputs, response_format etc.).
	// See https://learn.microsoft.com/azure/ai-services/openai/reference#rest-api-versioning.
	DefaultAPIVersion = "2024-12-01-preview"
)
