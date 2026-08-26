package inmemory

import "errors"

// Provider names the backend in [vectorstore capabilities].
const Provider = "InMemory"

// ErrMissingEmbeddingModel is returned when StoreConfig.EmbeddingModel is nil.
var ErrMissingEmbeddingModel = errors.New("inmemory: EmbeddingModel is required")
