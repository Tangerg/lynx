package messages

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"

	"github.com/Tangerg/lynx/ai/commons/content"
)

func mergeSystemMessages(messages []*SystemMessage) *SystemMessage {
	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return messages[0]
	}

	sb := strings.Builder{}
	metadata := make(map[string]any)

	for i, msg := range messages {
		if msg == nil {
			continue
		}
		sb.WriteString(msg.Text())
		if i < len(messages)-1 {
			sb.WriteString("\n\n")
		}
		maps.Copy(metadata, msg.Metadata())
	}

	return NewSystemMessage(sb.String(), metadata)
}

// MergeSystemMessages combines multiple SystemMessage instances into a single SystemMessage.
// Text content is concatenated with double newlines as separators.
// Metadata from all messages is merged, with later messages overwriting earlier ones for duplicate keys.
func MergeSystemMessages(messages []Message) *SystemMessage {
	systemMessages := make([]*SystemMessage, 0, len(messages))

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		systemMessage, ok := msg.(*SystemMessage)
		if !ok {
			continue
		}
		systemMessages = append(systemMessages, systemMessage)
	}

	return mergeSystemMessages(systemMessages)
}

func mergeUserMessages(messages []*UserMessage) *UserMessage {
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
		if msg == nil {
			continue
		}
		sb.WriteString(msg.Text())
		if i < len(messages)-1 {
			sb.WriteString("\n\n")
		}
		maps.Copy(metadata, msg.Metadata())
		media = append(media, msg.Media()...)
	}

	return NewUserMessage(sb.String(), media, metadata)
}

// MergeUserMessages combines multiple UserMessage instances into a single UserMessage.
// Text content is concatenated with double newlines, media content is combined into a single slice,
// and metadata is merged with later messages overwriting earlier ones for duplicate keys.
func MergeUserMessages(messages []Message) *UserMessage {
	userMessages := make([]*UserMessage, 0, len(messages))

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		userMessage, ok := msg.(*UserMessage)
		if !ok {
			continue
		}
		userMessages = append(userMessages, userMessage)
	}

	return mergeUserMessages(userMessages)
}

func mergeToolResponseMessages(messages []*ToolResponseMessage) (*ToolResponseMessage, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	if len(messages) == 1 {
		return messages[0], nil
	}

	metadata := make(map[string]any)
	responses := make([]*ToolResponse, 0)

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		responses = append(responses, msg.ToolResponses()...)
		maps.Copy(metadata, msg.Metadata())
	}

	return NewToolResponseMessage(responses, metadata)
}

// MergeToolResponseMessages combines multiple ToolResponseMessage instances into a single ToolResponseMessage.
// All tool responses are combined into a single slice and metadata is merged with later messages
// overwriting earlier ones for duplicate keys.
func MergeToolResponseMessages(messages []Message) (*ToolResponseMessage, error) {
	toolResponseMessages := make([]*ToolResponseMessage, 0, len(messages))

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		toolResponseMessage, ok := msg.(*ToolResponseMessage)
		if !ok {
			continue
		}
		toolResponseMessages = append(toolResponseMessages, toolResponseMessage)
	}

	return mergeToolResponseMessages(toolResponseMessages)
}

// ContainsType checks whether a slice of messages contains at least one message of the specified type.
// Returns false for empty slices or when all messages are nil.
func ContainsType(messages []Message, typ Type) bool {
	if len(messages) == 0 {
		return false
	}

	return slices.ContainsFunc(messages, func(m Message) bool {
		return m != nil && m.Type() == typ
	})
}

// IsLastOfType checks whether the last message in a slice has the specified type.
// Returns false for empty slices or when the last message is nil.
func IsLastOfType(messages []Message, typ Type) bool {
	return IsIndexOfType(messages, -1, typ)
}

// IsFirstOfType checks whether the first message in a slice has the specified type.
// Returns false for empty slices or when the first message is nil.
func IsFirstOfType(messages []Message, typ Type) bool {
	return IsIndexOfType(messages, 0, typ)
}

// IsIndexOfType checks whether the message at a specific index has the specified type.
// Supports both positive and negative indexing (-1 for last element).
// Returns false for out-of-bounds indices or nil messages.
func IsIndexOfType(messages []Message, index int, typ Type) bool {
	msg, ok := pkgSlices.At(messages, index)
	if !ok {
		return false
	}

	return msg != nil && msg.Type() == typ
}

// FilterByTypes filters messages by their types, returning only messages that match any of the specified types.
// Returns the original slice if no types are specified. Nil messages are automatically skipped and original message order is preserved.
func FilterByTypes(messages []Message, types ...Type) []Message {
	if len(types) == 0 {
		return messages
	}

	return Filter(messages, func(item Message) bool {
		return item != nil && slices.Contains(types, item.Type())
	})
}

// Filter filters messages using a custom predicate function.
// Returns an empty slice if no messages are provided or no messages match the predicate.
// Panics if the predicate function is nil.
func Filter(messages []Message, predicate func(item Message) bool) []Message {
	if predicate == nil {
		panic("FilterBy: predicate is nil")
	}

	if len(messages) == 0 {
		return make([]Message, 0)
	}

	msgs := make([]Message, 0, len(messages))
	for _, msg := range messages {
		ok := predicate(msg)
		if ok {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// MergeMessagesByType merges messages of the specified type using the appropriate merge function.
// Returns an error for unsupported message types.
func MergeMessagesByType(messages []Message, typ Type) (Message, error) {
	if typ.IsSystem() {
		return MergeSystemMessages(messages), nil
	}

	if typ.IsUser() {
		return MergeUserMessages(messages), nil
	}

	if typ.IsTool() {
		return MergeToolResponseMessages(messages)
	}

	return nil, fmt.Errorf("unsupported message type for merging: %s", typ.String())
}

// FilterNonNil filters out nil messages from the slice.
func FilterNonNil(messages []Message) []Message {
	return Filter(messages, func(item Message) bool {
		return item != nil
	})
}

type adjacentSameTypeMerger struct {
	messages []Message
	result   []Message
	start    int
}

func (a *adjacentSameTypeMerger) merge() []Message {
	for i := 1; i <= len(a.messages); i++ {
		if a.isGroupEnd(i) {
			a.compressCurrentGroup(i)
			a.start = i
		}
	}
	return a.result
}

func (a *adjacentSameTypeMerger) isGroupEnd(index int) bool {
	if index == len(a.messages) {
		return true
	}
	return a.messages[index].Type() != a.messages[a.start].Type()
}

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
		a.result = append(a.result, group...)
	}
}

// MergeAdjacentSameTypeMessages merges consecutive messages of the same type into single messages.
// Only adjacent messages with identical types are combined together.
// Non-adjacent messages or messages with different types remain separate.
//
// Example:
//
//	Input:  [UserMsg, UserMsg, SystemMsg, UserMsg, ToolMsg, ToolMsg]
//	Output: [MergedUserMsg, SystemMsg, UserMsg, MergedToolMsg]
func MergeAdjacentSameTypeMessages(messages []Message) []Message {
	validMessages := FilterNonNil(messages)
	if len(validMessages) <= 1 {
		return validMessages
	}

	merger := &adjacentSameTypeMerger{
		messages: validMessages,
		result:   make([]Message, 0, len(validMessages)),
	}

	return merger.merge()
}
