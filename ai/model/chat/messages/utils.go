package messages

import (
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/ai/commons/content"
)

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

// MergeUserMessages combines multiple UserMessage instances into a single UserMessage.
// This function is useful when you need to consolidate several user messages
// into one unified message while preserving all media content.
//
// The merging process works as follows:
//   - Text content: All message texts are concatenated with double newlines ("\n\n") as separators
//   - Media content: All media items from all messages are combined into a single slice
//   - Metadata: All metadata maps are merged, with later messages' metadata potentially overwriting
//     earlier ones if they share the same keys
//
// Parameters:
//   - messages: Variable number of UserMessage pointers to be merged
//
// Returns:
//   - nil if no messages are provided
//   - The original message if only one message is provided (optimization to avoid unnecessary processing)
//   - A new UserMessage containing the merged text, combined media, and merged metadata for multiple messages
//
// Example:
//
//	msg1 := NewUserMessage("Hello", []*content.Media{image1}, nil)
//	msg2 := NewUserMessage("How are you?", []*content.Media{image2}, nil)
//	merged := MergeUserMessages(msg1, msg2)
//	// Result: Text="Hello\n\nHow are you?", Media=[image1, image2]
func MergeUserMessages(messages ...*UserMessage) *UserMessage {
	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return messages[0]
	}

	sb := strings.Builder{}
	metadata := make(map[string]any)
	media := make([]*content.Media, 0)
	for i, msg := range messages {
		sb.WriteString(msg.Text())
		if i < len(messages)-1 {
			sb.WriteString("\n\n")
		}
		maps.Copy(metadata, msg.Metadata())
		media = append(media, msg.Media()...)
	}

	return NewUserMessage(sb.String(), media, metadata)
}

// ContainsType checks whether a slice of messages contains at least one message of the specified type.
// This function is useful for validating message collections or implementing conditional logic
// based on the presence of certain message types.
//
// The function performs a nil-safe check, ignoring any nil message pointers in the slice.
//
// Parameters:
//   - messages: Slice of Message interfaces to search through
//   - typ: The Type to search for within the messages
//
// Returns:
//   - true if at least one non-nil message of the specified type is found
//   - false if no messages of the specified type are found or if all messages are nil
//
// Example:
//
//	messages := []Message{systemMsg, userMsg, assistantMsg}
//	hasSystem := ContainsType(messages, System)  // returns true
//	hasTool := ContainsType(messages, Tool)  // returns false
func ContainsType(messages []Message, typ Type) bool {
	if len(messages) == 0 {
		return false
	}
	return slices.ContainsFunc(messages, func(m Message) bool {
		return m != nil && m.Type() == typ
	})
}

// IsLastOfType checks whether the last message in a slice has the specified type.
// This function is useful for validating conversation flow or implementing conditional logic
// based on the type of the most recent message in a conversation.
//
// The function performs a safe check for empty slices but does not check for nil messages.
// If the last message is nil, calling Type() will cause a panic.
//
// Parameters:
//   - messages: Slice of Message interfaces to check
//   - typ: The Type to match against the last message
//
// Returns:
//   - false if the messages slice is empty
//   - true if the last message's type matches the specified type
//   - false if the last message's type does not match the specified type
//
// Example:
//
//	messages := []Message{systemMsg, userMsg, assistantMsg}
//	isLastAssistant := IsLastMessageOfType(messages, Assistant)  // returns true
//	isLastUser := IsLastMessageOfType(messages, User)            // returns false
//	isEmpty := IsLastMessageOfType([]Message{}, User)            // returns false
func IsLastOfType(messages []Message, typ Type) bool {
	if len(messages) == 0 {
		return false
	}
	lastMsg := messages[len(messages)-1]
	return lastMsg != nil && lastMsg.Type() == typ
}

// IsFirstOfType checks whether the first message in a slice has the specified type.
// This function is useful for validating conversation flow or implementing conditional logic
// based on the type of the initial message in a conversation.
//
// The function performs a safe check for empty slices but does not check for nil messages.
// If the first message is nil, calling Type() will cause a panic.
//
// Parameters:
//   - messages: Slice of Message interfaces to check
//   - typ: The Type to match against the first message
//
// Returns:
//   - false if the messages slice is empty
//   - true if the first message's type matches the specified type
//   - false if the first message's type does not match the specified type
//
// Example:
//
//	msgs := []Message{systemMsg, userMsg, assistantMsg}
//	isFirstSystem := messages.IsFirstOfType(msgs, System)     // returns true
//	isFirstUser := messages.IsFirstOfType(msgs, User)         // returns false
//	isEmpty := messages.IsFirstOfType([]Message{}, System)    // returns false
func IsFirstOfType(messages []Message, typ Type) bool {
	if len(messages) == 0 {
		return false
	}
	firstMsg := messages[0]
	return firstMsg != nil && firstMsg.Type() == typ
}
