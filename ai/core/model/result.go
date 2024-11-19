package model

// ResultMetadata is a marker interface that can be implemented by any type
// intended to provide metadata for a result. As a marker interface, it does
// not define any methods, serving only to indicate that a type can be used
// as metadata for a result.
type ResultMetadata interface {
}

// Result is a generic interface representing a result with an output of type T
// and associated metadata of type M. The metadata provides additional information
// about the result and must implement the ResultMetadata interface.
//
// Type Parameters:
// - T: The type of the result's output.
// - M: The type of the result's metadata, which must implement the ResultMetadata interface.
//
// Methods:
//
// Output:
//
//	Output() T
//	Retrieves the result's output value of type T.
//	Returns:
//	- T: The output value of the result.
//
// Metadata:
//
//	Metadata() M
//	Retrieves the metadata associated with the result.
//	Returns:
//	- M: The metadata for the result.
//
// Example Implementation:
//
//	type MyResult[T any, M ResultMetadata] struct {
//	    output   T
//	    metadata M
//	}
//
//	func (r *MyResult[T, M]) Output() T {
//	    return r.output
//	}
//
//	func (r *MyResult[T, M]) Metadata() M {
//	    return r.metadata
//	}
type Result[T any, M ResultMetadata] interface {
	// Output returns the result's output of type T.
	Output() T

	// Metadata returns the result's metadata of type M.
	Metadata() M
}
