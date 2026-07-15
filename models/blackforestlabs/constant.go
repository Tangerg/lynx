package blackforestlabs

const (
	Provider = "BlackForestLabs"
)

const (
	OptionsKey = "blackforestlabs/options"

	// DefaultBaseURL is BFL's production endpoint.
	DefaultBaseURL = "https://api.bfl.ai/v1"

	// DefaultPollInterval / DefaultPollTimeout configure how Call waits
	// for an async generation to complete. BFL's typical render time
	// is 3–15s for flux-dev / flux-pro / flux-schnell.
	DefaultPollIntervalSeconds = 1
	DefaultPollTimeoutSeconds  = 120
)
