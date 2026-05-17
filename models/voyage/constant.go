package voyage

const (
	Provider = "Voyage"
)

const (
	OptionsKey = "lynx:ai:model:voyage_options"

	// DefaultBaseURL is Voyage AI's production REST endpoint. Override
	// via [ApiConfig.BaseURL] when proxying through an internal gateway.
	DefaultBaseURL = "https://api.voyageai.com/v1"
)
