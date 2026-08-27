package gladia

import "time"

const (
	Provider = "Gladia"
)

const (
	RequestExtensionKey  = "gladia/request"
	ResponseExtensionKey = "gladia/response"
	DefaultBaseURL       = "https://api.gladia.io/v2"
	DefaultPollInterval  = 2 * time.Second
	DefaultPollTimeout   = 30 * time.Minute
)

const (
	ModelSolaria3 = "solaria-3"
	ModelSolaria1 = "solaria-1"
)
