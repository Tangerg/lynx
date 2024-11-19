package middleware

// ResponseFormatKey is a constant key used to specify the desired response format
// in a request's metadata or configuration. It acts as a standard identifier
// for applications needing to define or query the response format.
const (
	ResponseFormatKey = "response_format"
)

// ChatRequestMode defines a string type representing the mode of a chat request.
// This type is used to differentiate between various processing modes for chat interactions.
type ChatRequestMode string

const (
	// CallRequest represents a chat request mode where the interaction is handled
	// as a single, synchronous operation, processing the input and returning
	// the complete result in one step.
	CallRequest ChatRequestMode = "call"

	// StreamRequest represents a chat request mode where the interaction is handled
	// as a streaming operation, processing the input incrementally and returning
	// results in real time.
	StreamRequest ChatRequestMode = "stream"
)
