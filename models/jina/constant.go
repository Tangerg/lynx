package jina

const (
	Provider = "Jina"

	OptionsKey = "jina/options"

	DefaultBaseURL = "https://api.jina.ai/v1"
)

// See https://jina.ai/embeddings/ for the current catalog.
const (
	ModelEmbeddingsV3       = "jina-embeddings-v3"
	ModelEmbeddingsV2BaseEN = "jina-embeddings-v2-base-en"
	ModelClipV2             = "jina-clip-v2"
	ModelColbertV2          = "jina-colbert-v2"
)
