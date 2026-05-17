package revai

import "time"

const (
	Provider = "RevAI"
)

const (
	OptionsKey          = "lynx:ai:model:revai_options"
	DefaultBaseURL      = "https://api.rev.ai/speechtotext/v1"
	DefaultPollInterval = 3 * time.Second
	DefaultPollTimeout  = 30 * time.Minute
)
