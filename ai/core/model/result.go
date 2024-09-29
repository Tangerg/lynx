package model

// ResultMetadata is an interface that can be implemented by any type
// that wants to provide metadata for a result. It doesn't specify any
// methods, so it's a marker interface.
type ResultMetadata interface {
}

// Result is a generic interface that represents a result with an output
// of type T and metadata of type M. The metadata type M must implement
// the ResultMetadata interface.
type Result[T any, M ResultMetadata] interface {
	// Output returns the result's output of type T.
	Output() T

	// Metadata returns the result's metadata of type M.
	Metadata() M
}
