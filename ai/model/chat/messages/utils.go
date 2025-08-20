package messages

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"

	"github.com/Tangerg/lynx/ai/content"
)

// ContainsType reports whether a slice of messages contains at least one message of the specified type.
//
// Parameters:
//   - messages: The slice of messages to search through
//   - typ: The message type to look for
//
// Returns:
//   - bool: True if at least one message of the specified type is found, false otherwise
//
// Note: Returns false for empty slices or when all messages are nil.
func ContainsType(messages []Message, typ Type) bool {
	if len(messages) == 0 {
		return false
	}

	return slices.ContainsFunc(messages, func(m Message) bool {
		return m != nil && m.Type() == typ
	})
}

// HasTypeAtLast reports whether the last message in a slice has the specified type.
//
// Parameters:
//   - messages: The slice of messages to check
//   - typ: The expected message type
//
// Returns:
//   - bool: True if the last message has the specified type, false otherwise
//
// Note: Returns false for empty slices or when the last message is nil.
func HasTypeAtLast(messages []Message, typ Type) bool {
	return HasTypeAt(messages, -1, typ)
}

// HasTypeAtFirst reports whether the first message in a slice has the specified type.
//
// Parameters:
//   - messages: The slice of messages to check
//   - typ: The expected message type
//
// Returns:
//   - bool: True if the first message has the specified type, false otherwise
//
// Note: Returns false for empty slices or when the first message is nil.
func HasTypeAtFirst(messages []Message, typ Type) bool {
	return HasTypeAt(messages, 0, typ)
}

// HasTypeAt reports whether the message at a specific index has the specified type.
// Supports both positive and negative indexing (-1 for last element, -2 for second-to-last, etc.).
//
// Parameters:
//   - messages: The slice of messages to check
//   - index: The index to check (supports negative indexing)
//   - typ: The expected message type
//
// Returns:
//   - bool: True if the message at the specified index has the expected type, false otherwise
//
// Note: Returns false for out-of-bounds indices or nil messages.
func HasTypeAt(messages []Message, index int, typ Type) bool {
	msg, ok := pkgSlices.At(messages, index)
	if !ok {
		return false
	}

	return msg != nil && msg.Type() == typ
}

// Filter creates a new slice containing only messages that satisfy the predicate function.
//
// Parameters:
//   - messages: The slice of messages to filter
//   - predicate: Function that returns true for messages to include in the result
//
// Returns:
//   - []Message: New slice containing only messages that match the predicate
//
// Note: Returns an empty slice if no messages are provided or no messages match the predicate.
// Panics if the predicate function is nil.
func Filter(messages []Message, predicate func(item Message) bool) []Message {
	if predicate == nil {
		panic("Filter: predicate function cannot be nil")
	}

	if len(messages) == 0 {
		return make([]Message, 0)
	}

	msgs := make([]Message, 0, len(messages))
	for _, msg := range messages {
		if predicate(msg) {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// FilterByTypes returns a new slice containing only messages that match any of the specified types.
// Nil messages are automatically excluded and the original message order is preserved.
//
// Parameters:
//   - messages: The slice of messages to filter
//   - types: Variable number of message types to include (System, User, Assistant, Tool)
//
// Returns:
//   - []Message: New slice containing only messages of the specified types
//
// Note: If no types are specified, returns the original slice unchanged.
//
// Example:
//
//	FilterByTypes(msgs, User, Assistant) // Returns only user and assistant messages
func FilterByTypes(messages []Message, types ...Type) []Message {
	if len(types) == 0 {
		return messages
	}

	return Filter(messages, func(item Message) bool {
		return item != nil && slices.Contains(types, item.Type())
	})
}

// ExcludeNil returns a new slice with all nil messages removed.
//
// Parameters:
//   - messages: The slice of messages to filter
//
// Returns:
//   - []Message: New slice with all nil messages removed
//
// Note: This function is useful for cleaning up message slices that may contain nil values.
func ExcludeNil(messages []Message) []Message {
	return Filter(messages, func(item Message) bool {
		return item != nil
	})
}

// MergeSystemMessages combines multiple SystemMessage instances into a single SystemMessage.
// Text content is concatenated with double newlines as separators, and metadata from all
// messages is merged with later messages overwriting earlier ones for duplicate keys.
//
// Parameters:
//   - messages: The slice of messages to filter and merge
//
// Returns:
//   - *SystemMessage: The merged system message, or nil if no system messages are found
func MergeSystemMessages(messages []Message) *SystemMessage {
	messages = FilterByTypes(messages, System)

	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return messages[0].(*SystemMessage)
	}

	sb := strings.Builder{}
	metadata := make(map[string]any)

	for _, msg := range messages {
		systemMessage := msg.(*SystemMessage)
		sb.WriteString(systemMessage.text)
		sb.WriteString("\n\n")
		maps.Copy(metadata, systemMessage.metadata)
	}

	return NewSystemMessage(
		MessageParams{
			Text:     strings.TrimSuffix(sb.String(), "\n\n"),
			Metadata: metadata,
		})
}

// MergeUserMessages combines multiple UserMessage instances into a single UserMessage.
// Text content is concatenated with double newlines, media content is combined into a single slice,
// and metadata is merged with later messages overwriting earlier ones for duplicate keys.
//
// Parameters:
//   - messages: The slice of messages to filter and merge
//
// Returns:
//   - *UserMessage: The merged user message, or nil if no user messages are found
func MergeUserMessages(messages []Message) *UserMessage {
	messages = FilterByTypes(messages, User)

	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return messages[0].(*UserMessage)
	}

	sb := strings.Builder{}
	metadata := make(map[string]any)
	media := make([]*content.Media, 0)

	for _, msg := range messages {
		userMessage := msg.(*UserMessage)
		sb.WriteString(userMessage.text)
		sb.WriteString("\n\n")
		maps.Copy(metadata, userMessage.metadata)
		media = append(media, userMessage.media...)
	}

	return NewUserMessage(MessageParams{
		Text:     strings.TrimSuffix(sb.String(), "\n\n"),
		Media:    media,
		Metadata: metadata,
	})
}

// MergeToolMessages combines multiple ToolMessage instances into a single ToolMessage.
// All tool returns are combined into a single slice and metadata is merged with later
// messages overwriting earlier ones for duplicate keys.
//
// Parameters:
//   - messages: The slice of messages to filter and merge
//
// Returns:
//   - *ToolMessage: The merged tool message, or nil if no tool messages are found
//   - error: Non-nil if the merge operation fails
func MergeToolMessages(messages []Message) (*ToolMessage, error) {
	messages = FilterByTypes(messages, Tool)

	if len(messages) == 0 {
		return nil, nil
	}
	if len(messages) == 1 {
		return messages[0].(*ToolMessage), nil
	}

	metadata := make(map[string]any)
	toolReturns := make([]*ToolReturn, 0)

	for _, msg := range messages {
		toolMessage := msg.(*ToolMessage)
		maps.Copy(metadata, toolMessage.metadata)
		toolReturns = append(toolReturns, toolMessage.toolReturns...)
	}

	return NewToolMessage(MessageParams{
		ToolReturns: toolReturns,
		Metadata:    metadata,
	})
}

// MergeMessagesByType merges messages of the specified type using the appropriate merge function.
// Each message type has its own merging strategy to preserve type-specific data.
//
// Parameters:
//   - messages: The slice of messages to merge
//   - typ: The message type to filter and merge (System, User, or Tool)
//
// Returns:
//   - Message: The merged message of the specified type, or nil if no messages of that type are found
//   - error: Non-nil for unsupported message types or merge failures
//
// Note: Assistant messages are not supported for merging as they typically represent individual
// AI responses that should remain separate to maintain conversation context.
func MergeMessagesByType(messages []Message, typ Type) (Message, error) {
	if typ.IsSystem() {
		return MergeSystemMessages(messages), nil
	}

	if typ.IsUser() {
		return MergeUserMessages(messages), nil
	}

	if typ.IsTool() {
		return MergeToolMessages(messages)
	}

	return nil, fmt.Errorf("unsupported message type for merging: %s", typ.String())
}

// adjacentSameTypeMerger handles the process of merging consecutive messages of the same type.
type adjacentSameTypeMerger struct {
	messages []Message // Source messages to process
	result   []Message // Resulting merged messages
	start    int       // Start index of current group being processed
}

// merge processes all messages and returns the result with adjacent same-type messages merged.
func (a *adjacentSameTypeMerger) merge() []Message {
	for i := 1; i <= len(a.messages); i++ {
		if a.isGroupEnd(i) {
			a.compressCurrentGroup(i)
			a.start = i
		}
	}
	return a.result
}

// isGroupEnd determines if the current group of same-type messages has ended.
func (a *adjacentSameTypeMerger) isGroupEnd(index int) bool {
	if index == len(a.messages) {
		return true
	}
	return a.messages[index].Type() != a.messages[a.start].Type()
}

// compressCurrentGroup merges the current group of messages if possible.
func (a *adjacentSameTypeMerger) compressCurrentGroup(end int) {
	group := a.messages[a.start:end]

	if len(group) == 1 {
		a.result = append(a.result, group[0])
		return
	}

	merged, err := MergeMessagesByType(group, group[0].Type())
	if err == nil {
		a.result = append(a.result, merged)
	} else {
		// If merging fails, keep original messages
		a.result = append(a.result, group...)
	}
}

// MergeAdjacentSameTypeMessages combines consecutive messages of the same type into single messages.
// Only adjacent messages with identical types are merged together, preserving the overall
// conversation structure while reducing redundancy.
//
// Parameters:
//   - messages: The slice of messages to process
//
// Returns:
//   - []Message: New slice with adjacent same-type messages merged
//
// Note: Non-adjacent messages or messages with different types remain separate.
// Nil messages are automatically filtered out before processing.
//
// Example:
//
//	Input:  [UserMsg, UserMsg, SystemMsg, UserMsg, ToolMsg, ToolMsg]
//	Output: [MergedUserMsg, SystemMsg, UserMsg, MergedToolMsg]
func MergeAdjacentSameTypeMessages(messages []Message) []Message {
	validMessages := ExcludeNil(messages)
	if len(validMessages) <= 1 {
		return validMessages
	}

	merger := &adjacentSameTypeMerger{
		messages: validMessages,
		result:   make([]Message, 0, len(validMessages)),
	}

	return merger.merge()
}

// FirstIndexOfType finds the first occurrence of a message with the specified type.
//
// Parameters:
//   - messages: The slice of messages to search through
//   - typ: The message type to search for
//
// Returns:
//   - int: The index of the first message with the specified type, or -1 if not found
//   - Message: The message at that index, or nil if not found
//
// Note: Automatically skips nil messages during the search.
func FirstIndexOfType(messages []Message, typ Type) (int, Message) {
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg != nil && msg.Type() == typ {
			return i, msg
		}
	}
	return -1, nil
}

// LastIndexOfType finds the last occurrence of a message with the specified type.
//
// Parameters:
//   - messages: The slice of messages to search through
//   - typ: The message type to search for
//
// Returns:
//   - int: The index of the last message with the specified type, or -1 if not found
//   - Message: The message at that index, or nil if not found
//
// Note: Automatically skips nil messages during the search.
func LastIndexOfType(messages []Message, typ Type) (int, Message) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg != nil && msg.Type() == typ {
			return i, msg
		}
	}
	return -1, nil
}

// AugmentLastMessageOfType finds the last message of the specified type and applies
// a transformation function to it. The transformation occurs in-place within the original slice.
//
// Parameters:
//   - messages: The slice of messages to modify (modified in-place)
//   - msgType: The type of message to find and transform
//   - transformFn: Function that takes the found message and returns a transformed version
//
// Note: If the transformation function returns nil, the original message remains unchanged.
// If no message of the specified type is found, no modification occurs.
func AugmentLastMessageOfType(messages []Message, msgType Type, transformFn func(message Message) Message) {
	if transformFn == nil {
		return
	}

	lastIndex, lastMsg := LastIndexOfType(messages, msgType)
	if lastIndex == -1 {
		return
	}

	transformedMsg := transformFn(lastMsg)
	if transformedMsg != nil {
		messages[lastIndex] = transformedMsg
	}
}

// AugmentTextLastMessageOfType appends additional text to the last message of the specified type.
// The text is appended with double newlines as separators. Only supports UserMessage and
// SystemMessage types as they contain modifiable text content.
//
// Parameters:
//   - messages: The slice of messages to modify (modified in-place)
//   - msgType: The type of message to find and augment (User or System)
//   - additionalText: The text to append to the found message
//
// Note: If the message type doesn't support text augmentation, no modification occurs.
// For unsupported message types (Assistant, Tool), the function silently does nothing.
func AugmentTextLastMessageOfType(messages []Message, msgType Type, additionalText string) {
	AugmentLastMessageOfType(messages, msgType, func(currentMsg Message) Message {
		switch typedMsg := currentMsg.(type) {
		case *UserMessage:
			typedMsg.text = typedMsg.text + "\n\n" + additionalText
			return typedMsg
		case *SystemMessage:
			typedMsg.text = typedMsg.text + "\n\n" + additionalText
			return typedMsg
		default:
			return typedMsg // Return unchanged for unsupported types
		}
	})
}
