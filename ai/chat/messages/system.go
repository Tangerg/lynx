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

// NewSystemMessage creates a new system message with the given text content.
//
// Optionally accepts metadata as a map. If multiple metadata maps are provided,
// only the first one will be used.
func NewSystemMessage(text string, metadata ...map[string]any) *SystemMessage {
	return &SystemMessage{
		message: newmessage(System, text, metadata...),
	}
}
