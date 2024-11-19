package model

// RequestOptions is a marker interface used to indicate types that can be utilized
// as options for a request.
type RequestOptions interface {
}

// Request is a generic interface that defines a structure for handling requests
// with associated instructions and options.
//
// Type Parameters:
//   - T: The type representing the instructions for the request.
//   - O: The type that implements the RequestOptions interface, representing
//     optional parameters for the request.
//
// Methods:
//
// Instructions:
//
//	Instructions() T
//	Retrieves the instructions associated with the request.
//	Returns:
//	- T: The instructions of the request.
//
// Options:
//
//	Options() O
//	Retrieves the options associated with the request. The options must
//	implement the RequestOptions interface.
//	Returns:
//	- O: The options for the request.
//
// Example Implementation:
//
//	type MyRequest struct {
//	    instructions string
//	    options      MyOptions
//	}
//
//	type MyOptions struct {
//	    Timeout time.Duration
//	}
//
//	func (o MyOptions) RequestOptions() {}
//
//	func (r MyRequest) Instructions() string {
//	    return r.instructions
//	}
//
//	func (r MyRequest) Options() MyOptions {
//	    return r.options
//	}
type Request[T any, O RequestOptions] interface {
	Instructions() T
	Options() O
}
