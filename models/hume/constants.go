package hume

// Provider is the stable backend name for host-side attribution.
const (
	Provider = "Hume"
)

// Exported identifiers keep provider-owned names and defaults out of caller literals.
const (
	SpeechRequestExtensionKey = "hume/speech_request"
	DefaultBaseURL            = "https://api.hume.ai/v0"
)

// These are the provider values this adapter recognizes.
const (
	ModelOctave1 = "1"
	ModelOctave2 = "2"
)
