package model

// Content interface defines a contract for types that can provide content and metadata.
type Content interface {
	// Content method returns the content as a string.
	Content() string

	// Metadata method returns a map containing metadata information.
	// The map keys are strings, and the values can be of any type.
	Metadata() map[string]any
}
