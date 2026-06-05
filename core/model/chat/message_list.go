package chat

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/media"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// hasMessageTypeAtLast reports whether the last message has expectedType.
// Empty slices and nil entries return false.
func hasMessageTypeAtLast(messages []Message, expectedType MessageType) bool {
	return hasMessageTypeAt(messages, -1, expectedType)
}

// hasMessageTypeAt reports whether the message at index has expectedType.
// Negative indexes are supported (-1 is the last entry).
func hasMessageTypeAt(messages []Message, index int, expectedType MessageType) bool {
	msg, exists := pkgSlices.At(messages, index)
	if !exists {
		return false
	}
	return msg != nil && msg.Type() == expectedType
}

// FilterMessages returns messages for which predicate returns true. The
// original order is preserved. Panics on a nil predicate — that's a
// programmer error, not a runtime condition.
func FilterMessages(messages []Message, predicate func(message Message) bool) []Message {
	if predicate == nil {
		panic("chat.FilterMessages: predicate must not be nil")
	}
	if len(messages) == 0 {
		return nil
	}

	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		if predicate(msg) {
			out = append(out, msg)
		}
	}
	return out
}

// FilterMessagesByMessageTypes returns messages whose type matches any of
// types. Nil entries are dropped. With no types supplied the input is
// returned unchanged.
func FilterMessagesByMessageTypes(messages []Message, types ...MessageType) []Message {
	if len(types) == 0 {
		return messages
	}

	return FilterMessages(messages, func(msg Message) bool {
		return msg != nil && slices.Contains(types, msg.Type())
	})
}

// filterOutNilMessages returns a copy with nil entries removed.
func filterOutNilMessages(messages []Message) []Message {
	return FilterMessages(messages, func(msg Message) bool { return msg != nil })
}

// MergeSystemMessages collapses every [SystemMessage] in messages into
// one. Text fields concatenate with double-newline separators; metadata
// merges last-write-wins. Returns nil when no system message exists.
func MergeSystemMessages(messages []Message) *SystemMessage {
	systems := FilterMessagesByMessageTypes(messages, MessageTypeSystem)

	if len(systems) == 0 {
		return nil
	}
	if len(systems) == 1 {
		return systems[0].(*SystemMessage)
	}

	var text strings.Builder
	merged := make(map[string]any)

	for _, m := range systems {
		s := m.(*SystemMessage)
		text.WriteString(s.Text)
		text.WriteString("\n\n")
		maps.Copy(merged, s.Metadata)
	}

	return NewSystemMessage(MessageParams{
		Text:     strings.TrimSuffix(text.String(), "\n\n"),
		Metadata: merged,
	})
}

// MergeUserMessages collapses every [UserMessage] in messages into one.
func MergeUserMessages(messages []Message) *UserMessage {
	users := FilterMessagesByMessageTypes(messages, MessageTypeUser)

	if len(users) == 0 {
		return nil
	}
	if len(users) == 1 {
		return users[0].(*UserMessage)
	}

	var text strings.Builder
	merged := make(map[string]any)
	var mergedMedia []*media.Media

	for _, m := range users {
		u := m.(*UserMessage)
		text.WriteString(u.Text)
		text.WriteString("\n\n")
		maps.Copy(merged, u.Metadata)
		mergedMedia = append(mergedMedia, u.Media...)
	}

	return NewUserMessage(MessageParams{
		Text:     strings.TrimSuffix(text.String(), "\n\n"),
		Media:    mergedMedia,
		Metadata: merged,
	})
}

// MergeToolMessages collapses every [ToolMessage] in messages into one.
func MergeToolMessages(messages []Message) (*ToolMessage, error) {
	tools := FilterMessagesByMessageTypes(messages, MessageTypeTool)

	if len(tools) == 0 {
		return nil, nil
	}
	if len(tools) == 1 {
		return tools[0].(*ToolMessage), nil
	}

	merged := make(map[string]any)
	var mergedReturns []*ToolReturn

	for _, m := range tools {
		tm := m.(*ToolMessage)
		maps.Copy(merged, tm.Metadata)
		mergedReturns = append(mergedReturns, tm.ToolReturns...)
	}

	return NewToolMessage(MessageParams{
		ToolReturns: mergedReturns,
		Metadata:    merged,
	})
}

// MergeMessages dispatches to the right per-type merge helper. Assistant
// messages cannot be merged — each represents a distinct model turn.
func MergeMessages(messages []Message, messageType MessageType) (Message, error) {
	switch messageType {
	case MessageTypeSystem:
		return MergeSystemMessages(messages), nil
	case MessageTypeUser:
		return MergeUserMessages(messages), nil
	case MessageTypeTool:
		return MergeToolMessages(messages)
	default:
		return nil, fmt.Errorf("chat.MergeMessages: cannot merge type %q", messageType)
	}
}

// MergeAdjacentSameTypeMessages folds each run of consecutive same-type
// messages into one merged message. Non-adjacent runs and runs of size 1
// are passed through unchanged.
func MergeAdjacentSameTypeMessages(messages []Message) []Message {
	source := filterOutNilMessages(messages)
	if len(source) <= 1 {
		return source
	}

	result := make([]Message, 0, len(source))
	groupStart := 0
	for i := 1; i <= len(source); i++ {
		if i < len(source) && source[i].Type() == source[groupStart].Type() {
			continue
		}
		group := source[groupStart:i]
		if len(group) == 1 {
			result = append(result, group[0])
		} else if merged, err := MergeMessages(group, group[0].Type()); err == nil {
			result = append(result, merged)
		} else {
			result = append(result, group...)
		}
		groupStart = i
	}
	return result
}

// findLastMessageIndexOfType returns the (index, message) of the last
// non-nil entry whose type equals targetType, or (-1, nil) when no such
// message exists.
func findLastMessageIndexOfType(messages []Message, targetType MessageType) (int, Message) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg != nil && msg.Type() == targetType {
			return i, msg
		}
	}
	return -1, nil
}

// augmentLastMessageOfType finds the last message of targetType and
// runs augmentFunc on it in place.
func augmentLastMessageOfType(messages []Message, targetType MessageType, augmentFunc func(message Message) Message) {
	if augmentFunc == nil {
		return
	}

	idx, last := findLastMessageIndexOfType(messages, targetType)
	if idx == -1 {
		return
	}

	if augmented := augmentFunc(last); augmented != nil {
		messages[idx] = augmented
	}
}

// appendTextToLastMessageOfType appends text to the last user or system
// message, separated by a double newline. Other types are silently
// ignored.
func appendTextToLastMessageOfType(messages []Message, targetType MessageType, text string) {
	augmentLastMessageOfType(messages, targetType, func(msg Message) Message {
		switch typed := msg.(type) {
		case *UserMessage:
			typed.Text = typed.Text + "\n\n" + text
			return typed
		case *SystemMessage:
			typed.Text = typed.Text + "\n\n" + text
			return typed
		default:
			return typed
		}
	})
}

// replaceTextOfLastMessageOfType overwrites the text of the last user or
// system message.
func replaceTextOfLastMessageOfType(messages []Message, targetType MessageType, text string) {
	augmentLastMessageOfType(messages, targetType, func(msg Message) Message {
		switch typed := msg.(type) {
		case *UserMessage:
			typed.Text = text
			return typed
		case *SystemMessage:
			typed.Text = text
			return typed
		default:
			return typed
		}
	})
}

// MessageToString renders one message as "role: payload". For assistant
// messages, text parts are emitted verbatim followed by any tool calls
// as compact JSON.
func MessageToString(message Message) string {
	var b strings.Builder
	b.WriteString(message.Type().String())
	b.WriteString(": ")

	switch typed := message.(type) {
	case *UserMessage:
		b.WriteString(typed.Text)
	case *SystemMessage:
		b.WriteString(typed.Text)
	case *AssistantMessage:
		b.WriteString(typed.JoinedText())
		if typed.HasToolCalls() {
			b.WriteByte('\n')
			calls := typed.CollectToolCalls()
			data, _ := json.Marshal(calls)
			b.Write(data)
		}
	case *ToolMessage:
		returns, _ := json.Marshal(typed.ToolReturns)
		b.Write(returns)
	}
	return b.String()
}

// MessagesToStrings maps [MessageToString] over messages.
func MessagesToStrings(messages []Message) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, MessageToString(msg))
	}
	return out
}
