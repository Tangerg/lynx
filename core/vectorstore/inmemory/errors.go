package inmemory

import "errors"

// Provider names the backend in [vectorstore capabilities].
const Provider = "InMemory"

var ErrMissingEmbeddingModel = errors.New("inmemory: embedding model is required")
