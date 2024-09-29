package model

// ResponseMetadata is an interface that defines methods for accessing
// metadata associated with a response. It provides methods to get
// metadata values by key.
type ResponseMetadata interface {
	// Get retrieves the value associated with the given key. It returns
	// the value and a boolean indicating whether the key was found.
	Get(key string) (any, bool)

	// GetOrDefault retrieves the value associated with the given key.
	// If the key is not found, it returns the provided default value.
	GetOrDefault(key string, def any) any
}

// Response is a generic interface that represents a response containing
// a result and associated metadata. The result is of type T and the
// metadata is of type M, which must implement the ResultMetadata interface.
type Response[T any, M ResultMetadata] interface {
	// Result returns a single result of type Result[T, M].
	Result() Result[T, M]

	// Results returns multiple results of type Result[T, M].
	Results() []Result[T, M]

	// Metadata returns the metadata associated with the response,
	// implementing the ResponseMetadata interface.
	Metadata() ResponseMetadata
}
