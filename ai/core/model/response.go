package model

// ResponseMetadata defines an interface for accessing metadata associated
// with a response. This interface allows retrieval of metadata values
// using specific keys.
//
// Implementations of this interface typically store metadata as key-value pairs
// and provide methods for querying or iterating over these pairs.
//
// Example Usage:
//
//	type MyResponseMetadata struct {
//	    data map[string]string
//	}
//
//	func (m *MyResponseMetadata) Get(key string) (string, bool) {
//	    value, exists := m.data[key]
//	    return value, exists
//	}
type ResponseMetadata interface {
}

// Response is a generic interface representing a response that contains a result
// and associated metadata. This interface is designed for handling both single
// and multiple results, along with metadata describing the response.
//
// Type Parameters:
// - T: The type of the result(s) contained in the response.
// - M: The type of the result's metadata, which must implement the ResultMetadata interface.
//
// Methods:
//
// Result:
//
//	Result() Result[T, M]
//	Retrieves a single result of type Result[T, M] from the response.
//	Returns:
//	- Result[T, M]: The single result associated with the response.
//
// Results:
//
//	Results() []Result[T, M]
//	Retrieves multiple results of type Result[T, M] from the response.
//	Returns:
//	- []Result[T, M]: A slice of results associated with the response.
//
// Metadata:
//
//	Metadata() ResponseMetadata
//	Retrieves the metadata associated with the response. The metadata provides
//	additional information about the response and implements the ResponseMetadata interface.
//	Returns:
//	- ResponseMetadata: The metadata for the response.
//
// Example Implementation:
//
//	type MyResponse[T any, M ResultMetadata] struct {
//	    results  []Result[T, M]
//	    metadata ResponseMetadata
//	}
//
//	func (r *MyResponse[T, M]) Result() Result[T, M] {
//	    return r.results[0]
//	}
//
//	func (r *MyResponse[T, M]) Results() []Result[T, M] {
//	    return r.results
//	}
//
//	func (r *MyResponse[T, M]) Metadata() ResponseMetadata {
//	    return r.metadata
//	}
type Response[T any, M ResultMetadata] interface {
	// Result returns a single result of type Result[T, M].
	Result() Result[T, M]

	// Results returns multiple results of type Result[T, M].
	Results() []Result[T, M]

	// Metadata returns the metadata associated with the response,
	// implementing the ResponseMetadata interface.
	Metadata() ResponseMetadata
}
