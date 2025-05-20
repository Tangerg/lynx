package id

// Generator defines an interface for generating unique identifiers.
// Implementations can create IDs based on various strategies and input objects.
type Generator interface {
	// GenerateId creates a unique identifier string based on optional input objects.
	// The implementation determines how the input objects influence the generated ID.
	//
	// Parameters:
	//   obj - one or more objects that may be used in the ID generation process
	//
	// Returns:
	//   A string representing the generated unique identifier
	GenerateId(obj ...any) string
}
