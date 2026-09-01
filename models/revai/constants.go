package revai

import "time"

// Provider is the stable backend name for host-side attribution.
const (
	Provider = "RevAI"
)

// Exported identifiers keep provider-owned names and defaults out of caller literals.
const (
	RequestExtensionKey  = "revai/request"
	ResponseExtensionKey = "revai/response"
	DefaultBaseURL       = "https://api.rev.ai/speechtotext/v1"
	DefaultPollInterval  = 3 * time.Second
	DefaultPollTimeout   = 30 * time.Minute
)

// These are the provider values this adapter recognizes.
const (
	ModelMachine = "machine"
	ModelHuman   = "human"
)
