package openrouter

const (
	Provider = "OpenRouter"
)

const (
	// BaseURL is OpenRouter's production endpoint.
	BaseURL = "https://openrouter.ai/api/v1"
)

const (
	// HeaderReferer is the standard HTTP Referer header used by
	// OpenRouter for app attribution / analytics. Pass your app's
	// homepage URL.
	HeaderReferer = "HTTP-Referer"

	// HeaderAppTitle is the X-Title header OpenRouter shows on
	// leaderboards / rankings. Pass your app's display name.
	HeaderAppTitle = "X-Title"
)
