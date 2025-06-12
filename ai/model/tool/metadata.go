package tool

// Metadata represents metadata about a tool specification and execution.
// It contains configuration that affects how the framework handles tool results.
type Metadata struct {
	// ReturnDirect indicates whether the tool result should be returned directly
	// to the user or passed back to the AI model for further processing.
	//
	// - true: Return result directly to user (typical for external tools like user interactions)
	// - false: Pass result back to AI model for further processing (typical for internal tools)
	returnDirect bool
}

// ReturnDirect returns whether the tool result should be returned directly
// to the user or passed back to the AI model for further processing.
func (m *Metadata) ReturnDirect() bool {
	return m.returnDirect
}

// NewMetadata creates a new Metadata instance with the specified returnDirect setting.
//
// Parameters:
//   - returnDirect: Whether tool results should be returned directly to the user
//
// Returns:
//   - *Metadata: A new Metadata instance
func NewMetadata(returnDirect bool) *Metadata {
	return &Metadata{
		returnDirect: returnDirect,
	}
}
