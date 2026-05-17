package gladia

import "time"

const (
	Provider = "Gladia"
)

const (
	OptionsKey          = "lynx:ai:model:gladia_options"
	DefaultBaseURL      = "https://api.gladia.io/v2"
	DefaultPollInterval = 2 * time.Second
	DefaultPollTimeout  = 30 * time.Minute
)
