package deepgram

// Provider is the stable backend name for host-side attribution.
const (
	Provider = "Deepgram"
)

// Exported identifiers keep provider-owned names and defaults out of caller literals.
const (
	SpeechRequestExtensionKey         = "deepgram/speech_request"
	SpeechResponseExtensionKey        = "deepgram/speech_response"
	TranscriptionRequestExtensionKey  = "deepgram/transcription_request"
	TranscriptionResponseExtensionKey = "deepgram/transcription_response"

	// DefaultBaseURL is Deepgram's production REST endpoint.
	DefaultBaseURL = "https://api.deepgram.com/v1"
)
