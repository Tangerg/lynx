package tool

// Metadata represents immutable metadata about tool specification and execution behavior.
// Once created, the metadata cannot be modified, ensuring consistent tool behavior
// across LLM interactions and maintaining thread safety in concurrent environments.
//
// Controls how the LLM framework processes tool execution results.
type Metadata struct {
	// ReturnDirect is an immutable flag indicating whether tool results should bypass
	// further LLM processing and be returned directly to the user.
	//
	// - true: Return result directly to user (e.g., user interaction tools, final output tools)
	// - false: Pass result back to LLM for integration and further processing (default behavior)
	returnDirect bool
}

// ReturnDirect returns the immutable setting for direct result handling.
// This value cannot be changed after the Metadata instance is created.
//
// Returns:
//   - bool: true if tool results should bypass LLM and go directly to user
func (m *Metadata) ReturnDirect() bool {
	return m.returnDirect
}

// NewMetadata creates an immutable Metadata instance with the specified behavior.
// Once created, the metadata configuration cannot be modified, ensuring consistent
// tool execution behavior throughout the application lifecycle.
//
// The returnDirect setting affects LLM workflow:
//   - true: Tool result becomes the final response, bypassing further LLM processing
//   - false: Tool result is fed back to LLM for integration into the conversation
//
// Parameters:
//   - returnDirect: Immutable flag for direct result handling behavior
//
// Returns:
//   - *Metadata: Immutable metadata instance with thread-safe access
func NewMetadata(returnDirect bool) *Metadata {
	return &Metadata{
		returnDirect: returnDirect,
	}
}
