package model

// RequestOptions is a marker interface used to indicate types that can be used as options.
// Marker interfaces do not contain any methods and are used to convey metadata about a type.
type RequestOptions interface {
}

// Request is a generic interface that defines a structure for handling requests with instructions and options.
// T represents the type of instructions, and O represents the type that implements the RequestOptions interface.
// The interface includes two methods:
// - Instructions() T: Returns the instructions of type T.
// - RequestOptions() O: Rxeturns the options of type O.
type Request[T any, O RequestOptions] interface {
	Instructions() T
	Options() O
}
