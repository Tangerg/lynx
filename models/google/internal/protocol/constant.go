package protocol

const DefaultBaseURL = "https://generativelanguage.googleapis.com"

const (
	SpeechRequestExtensionKey         = "google/speech_request"
	SpeechResponseExtensionKey        = "google/speech_response"
	TranscriptionRequestExtensionKey  = "google/transcription_request"
	TranscriptionResponseExtensionKey = "google/transcription_response"
	EmbeddingRequestExtensionKey      = "google/embedding_request"
	EmbeddingResponseExtensionKey     = "google/embedding_response"
	ImageRequestExtensionKey          = "google/image_request"
	ImageResponseExtensionKey         = "google/image_response"
)

const (
	ModelGemini36Flash      = "gemini-3.6-flash"
	ModelGemini35Flash      = "gemini-3.5-flash"
	ModelGemini35FlashLite  = "gemini-3.5-flash-lite"
	ModelGemini31ProPreview = "gemini-3.1-pro-preview"

	ModelGemini25FlashPreviewTTS = "gemini-2.5-flash-preview-tts"
	ModelGemini25ProPreviewTTS   = "gemini-2.5-pro-preview-tts"
	ModelGemini31FlashTTSPreview = "gemini-3.1-flash-tts-preview"

	ModelGemini25FlashImage     = "gemini-2.5-flash-image"
	ModelGemini3ProImage        = "gemini-3-pro-image"
	ModelGemini31FlashImage     = "gemini-3.1-flash-image"
	ModelGemini31FlashLiteImage = "gemini-3.1-flash-lite-image"

	ModelGeminiEmbedding2 = "gemini-embedding-2"
)
