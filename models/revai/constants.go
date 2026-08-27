package revai

import "time"

const (
	Provider = "RevAI"
)

const (
	RequestExtensionKey  = "revai/request"
	ResponseExtensionKey = "revai/response"
	DefaultBaseURL       = "https://api.rev.ai/speechtotext/v1"
	DefaultPollInterval  = 3 * time.Second
	DefaultPollTimeout   = 30 * time.Minute
)

const (
	ModelMachine = "machine"
	ModelHuman   = "human"
)
