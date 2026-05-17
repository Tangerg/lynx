package stability

const (
	Provider = "StabilityAI"
)

const (
	OptionsKey = "lynx:ai:model:stability_options"

	// DefaultBaseURL is Stability AI's v2beta REST endpoint. The older
	// v1 endpoint (api.stability.ai/v1/generation/...) is still alive
	// but v2beta is the current path for Stable Image / SD3 / Core /
	// Ultra calls.
	DefaultBaseURL = "https://api.stability.ai/v2beta"

	// EndpointCore picks the Stable Image Core engine — fastest /
	// cheapest tier.
	EndpointCore = "/stable-image/generate/core"

	// EndpointUltra picks the Stable Image Ultra engine — highest
	// quality.
	EndpointUltra = "/stable-image/generate/ultra"

	// EndpointSD3 picks Stable Diffusion 3 / 3.5.
	EndpointSD3 = "/stable-image/generate/sd3"

	// ResponseModeImage requests raw image bytes in the response body.
	ResponseModeImage = "image/*"

	// ResponseModeJSON requests a JSON envelope holding base64 bytes plus
	// the finish reason and the seed actually used (only reachable in
	// JSON mode).
	ResponseModeJSON = "application/json"
)
