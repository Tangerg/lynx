package lmnt

// Provider is the stable backend name for host-side attribution.
const (
	Provider = "LMNT"
)

// Exported identifiers keep provider-owned names and defaults out of caller literals.
const (
	RequestExtensionKey  = "lmnt/request"
	ResponseExtensionKey = "lmnt/response"
	DefaultBaseURL       = "https://api.lmnt.com/v1"
	CurrentAPIVersion    = "1.2"
	ModelBlizzard        = "blizzard"
)
