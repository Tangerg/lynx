package voyage

const (
	Provider = "Voyage"
)

const (
	EmbeddingRequestExtensionKey = "voyage/embedding_request"

	// DefaultBaseURL is Voyage AI's production REST endpoint. Override
	// via [APIConfig.BaseURL] when proxying through an internal gateway.
	DefaultBaseURL = "https://api.voyageai.com/v1"

	Model4Large = "voyage-4-large"
	Model4      = "voyage-4"
	Model4Lite  = "voyage-4-lite"
)
