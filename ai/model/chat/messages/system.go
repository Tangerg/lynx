package messages

var _ Message = (*SystemMessage)(nil)

// SystemMessage represents a system message containing high-level instructions
// for the conversation, such as behavior guidelines or response format requirements.
// This role typically provides high-level instructions for the conversation.
// For example, you might use a system message to instruct the AI to behave like
// a certain character or to provide answers in a specific format.
type SystemMessage struct {
	message
}

type SystemMessageParam struct {
	Text     string
	Metadata map[string]any
}

// NewSystemMessage creates a new system message using Go generics to simulate function overloading.
// This allows creating system messages with different parameter types in a single function call.
//
// Supported parameter types:
//   - string: Sets the text content
//   - SystemMessageParam: Complete parameter struct with text and metadata fields
//
// The function uses type constraints and type switching to handle different input types,
// providing a convenient API that mimics function overloading found in other languages.
//
// Examples:
//
//	NewSystemMessage("You are a helpful assistant")      // Text only
//	NewSystemMessage(SystemMessageParam{...})            // Full configuration
func NewSystemMessage[T string | SystemMessageParam](param T) *SystemMessage {
	var p SystemMessageParam

	switch typedParam := any(param).(type) {
	case string:
		p.Text = typedParam
	case SystemMessageParam:
		p = typedParam
	}

	return &SystemMessage{
		message: newMessage(System, p.Text, p.Metadata),
	}
}
