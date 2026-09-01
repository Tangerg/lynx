package luma

// Provider is the stable backend name for host-side attribution.
const (
	Provider = "Luma"
)

// Exported identifiers keep provider-owned names and defaults out of caller literals.
const (
	ImageRequestExtensionKey   = "luma/image_request"
	ResponseExtensionKey       = "luma/response"
	DefaultBaseURL             = "https://agents.lumalabs.ai/v1"
	DefaultPollIntervalSeconds = 2
	DefaultPollTimeoutSeconds  = 300
	ModelUni1                  = "uni-1"
	ModelUni1Max               = "uni-1-max"
)
