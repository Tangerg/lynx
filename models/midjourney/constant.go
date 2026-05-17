package midjourney

import "time"

const (
	Provider = "Midjourney"
)

const (
	OptionsKey          = "lynx:ai:model:midjourney_options"
	DefaultPollInterval = 5 * time.Second
	DefaultPollTimeout  = 10 * time.Minute
)
