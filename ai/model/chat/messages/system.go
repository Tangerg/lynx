package messages

import (
	"maps"
	"strings"
)

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

// MergeSystemMessages combines multiple SystemMessage instances into a single SystemMessage.
// This function is useful when you need to consolidate several system-level instructions
// or configuration messages into one unified message.
//
// The merging process works as follows:
//   - Text content: All message texts are concatenated with double newlines ("\n\n") as separators
//   - Metadata: All metadata maps are merged, with later messages' metadata potentially overwriting
//     earlier ones if they share the same keys
//
// Parameters:
//   - messages: Variable number of SystemMessage pointers to be merged
//
// Returns:
//   - nil if no messages are provided
//   - The original message if only one message is provided (optimization to avoid unnecessary processing)
//   - A new SystemMessage containing the merged content and metadata for multiple messages
//
// Example:
//
//	msg1 := NewSystemMessage("You are a helpful assistant.")
//	msg2 := NewSystemMessage("Please respond in a professional tone.")
//	merged := MergeSystemMessages(msg1, msg2)
//	// Result: "You are a helpful assistant.\n\nPlease respond in a professional tone."
func MergeSystemMessages(messages ...*SystemMessage) *SystemMessage {
	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return messages[0]
	}

	sb := strings.Builder{}
	metadata := make(map[string]any)

	for i, msg := range messages {
		sb.WriteString(msg.Text())
		if i < len(messages)-1 {
			sb.WriteString("\n\n")
		}
		maps.Copy(metadata, msg.Metadata())
	}
	return NewSystemMessage(sb.String(), metadata)
}
