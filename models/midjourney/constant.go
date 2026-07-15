package midjourney

import "time"

const (
	Provider = "Midjourney"
)

const (
	OptionsKey          = "midjourney/options"
	DefaultPollInterval = 5 * time.Second
	DefaultPollTimeout  = 10 * time.Minute
)
