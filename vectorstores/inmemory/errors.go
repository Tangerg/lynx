package inmemory

import "errors"

// Provider names the backend in [vectorstore.StoreInfo].
const Provider = "InMemory"

// Sentinel errors used by [NewStore] / config validation.
var (
	// ErrNilConfig is returned by NewStore when StoreConfig is nil.
	ErrNilConfig = errors.New("inmemory: config must not be nil")

	// ErrMissingEmbeddingClient is returned when EmbeddingClient is nil.
	ErrMissingEmbeddingClient = errors.New("inmemory: EmbeddingClient is required")
)
