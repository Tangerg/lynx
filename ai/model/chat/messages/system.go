package messages

var _ Message = (*SystemMessage)(nil)

// SystemMessage represents a system message that provides high-level instructions
// and context for the AI conversation. System messages typically contain behavior
// guidelines, response format requirements, role definitions, or conversational rules.
//
// System messages are processed before user messages and help establish the AI's
// behavior and response style. For example, you might use a system message to
// instruct the AI to behave like a specific character, follow particular guidelines,
// or provide answers in a designated format.
type SystemMessage struct {
	message
}

func (m *SystemMessage) Type() Type {
	return System
}

// NewSystemMessage creates a new system message using Go generics for type-safe parameter handling.
// This function provides a flexible API that accepts different parameter types to construct
// system messages with varying content configurations.
//
// Supported parameter types:
//   - string: Sets the text content of the system message
//   - MessageParams: Complete parameter struct containing text content and metadata
//
// The function uses type constraints and type switching to handle different input types,
// providing a convenient API for creating system messages with minimal boilerplate code.
//
// Examples:
//
//	NewSystemMessage("You are a helpful assistant")      // Creates system message with text only
//	NewSystemMessage(MessageParams{                       // Creates system message with full configuration
//	    Text: "You are a helpful assistant",
//	    Metadata: map[string]any{"priority": "high"},
//	})
//
// Note: System messages are typically placed at the beginning of a conversation
// to establish the AI's behavior and context for subsequent interactions.
func NewSystemMessage[T string | MessageParams](param T) *SystemMessage {
	var p MessageParams

	switch typedParam := any(param).(type) {
	case string:
		p.Text = typedParam
	case MessageParams:
		p = typedParam
	}

	return &SystemMessage{
		message: newMessage(p.Text, p.Metadata),
	}
}
