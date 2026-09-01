package stability

// Provider is the stable backend name for host-side attribution.
const (
	Provider = "StabilityAI"
)

// Exported identifiers keep provider-owned names and defaults out of caller literals.
const (
	RequestExtensionKey  = "stability/request"
	ResponseExtensionKey = "stability/response"

	// DefaultBaseURL is Stability AI's current Stable Image REST endpoint.
	DefaultBaseURL = "https://api.stability.ai/v2beta"

	// ResponseModeImage requests raw image bytes in the response body.
	ResponseModeImage = "image/*"

	// ResponseModeJSON requests a JSON envelope holding base64 bytes plus
	// the finish reason and the seed actually used (only reachable in
	// JSON mode).
	ResponseModeJSON = "application/json"
)

// These are the provider values this adapter recognizes.
const (
	ModelCore                = "stable-image-core"
	ModelUltra               = "stable-image-ultra"
	ModelSD3Point5Large      = "sd3.5-large"
	ModelSD3Point5LargeTurbo = "sd3.5-large-turbo"
	ModelSD3Point5Medium     = "sd3.5-medium"
	ModelSD3Point5Flash      = "sd3.5-flash"
)

const (
	endpointCore  = "/stable-image/generate/core"
	endpointUltra = "/stable-image/generate/ultra"
	endpointSD3   = "/stable-image/generate/sd3"
)
