package model

// Request is a generic interface that defines a structure for handling requests with instructions and options.
// T represents the type of instructions, and O represents the type that implements the Options interface.
// The interface includes two methods:
// - Instructions() T: Returns the instructions of type T.
// - Options() O: Returns the options of type O.
type Request[T any, O Options] interface {
	Instructions() T
	Options() O
}
