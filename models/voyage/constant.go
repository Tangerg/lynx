package voyage

const (
	Provider = "Voyage"
)

const (
	OptionsKey = "voyage/options"

	// DefaultBaseURL is Voyage AI's production REST endpoint. Override
	// via [APIConfig.BaseURL] when proxying through an internal gateway.
	DefaultBaseURL = "https://api.voyageai.com/v1"
)
