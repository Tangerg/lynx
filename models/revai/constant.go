package revai

import "time"

const (
	Provider = "RevAI"
)

const (
	OptionsKey          = "revai/options"
	DefaultBaseURL      = "https://api.rev.ai/speechtotext/v1"
	DefaultPollInterval = 3 * time.Second
	DefaultPollTimeout  = 30 * time.Minute
)
