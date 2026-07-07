package vectorstore

// Store is the union of [Creator], [Retriever], and [Deleter]
// plus a [Store.Metadata] accessor for provider identity. Concrete
// provider implementations live outside this interface package.
type Store interface {
	Creator
	Retriever
	Deleter

	// Metadata returns identity metadata about this store implementation.
	Metadata() StoreMetadata
}

// StoreMetadata holds identity metadata for a [Store]. NativeClient
// gives callers access to provider-specific operations the framework
// doesn't surface.
type StoreMetadata struct {
	NativeClient any    `json:"-"`
	Provider     string `json:"provider,omitempty"`
}
