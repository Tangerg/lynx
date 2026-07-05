package chat

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/media"
)

// MessageList is a conversation slice with protocol-level operations attached.
type MessageList []Message

// Filter returns messages for which predicate returns true. The original order
// is preserved. Panics on a nil predicate — that's a programmer error, not a
// runtime condition.
func (l MessageList) Filter(predicate func(message Message) bool) MessageList {
	if predicate == nil {
		panic("chat.MessageList.Filter: predicate must not be nil")
	}
	if len(l) == 0 {
		return nil
	}

	out := make(MessageList, 0, len(l))
	for _, msg := range l {
		if predicate(msg) {
			out = append(out, msg)
		}
	}
	return out
}

// FilterTypes returns messages whose type matches any of types. Nil entries are
// dropped. With no types supplied the input is returned unchanged.
func (l MessageList) FilterTypes(types ...MessageType) MessageList {
	if len(types) == 0 {
		return l
	}

	return l.Filter(func(msg Message) bool {
		return msg != nil && slices.Contains(types, msg.Type())
	})
}

func (l MessageList) withoutNil() MessageList {
	return l.Filter(func(msg Message) bool { return msg != nil })
}

// MergeSystem collapses every [SystemMessage] into one. Text fields concatenate
// with double-newline separators; metadata merges last-write-wins. Returns nil
// when no system message exists.
func (l MessageList) MergeSystem() *SystemMessage {
	systems := l.FilterTypes(MessageTypeSystem)

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

func (l MessageList) MergeUser() *UserMessage {
	users := l.FilterTypes(MessageTypeUser)

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

func (l MessageList) MergeTool() (*ToolMessage, error) {
	tools := l.FilterTypes(MessageTypeTool)

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

// Merge dispatches to the right per-type merge helper. Assistant messages
// cannot be merged — each represents a distinct model turn.
func (l MessageList) Merge(messageType MessageType) (Message, error) {
	switch messageType {
	case MessageTypeSystem:
		return l.MergeSystem(), nil
	case MessageTypeUser:
		return l.MergeUser(), nil
	case MessageTypeTool:
		return l.MergeTool()
	default:
		return nil, fmt.Errorf("chat.MessageList.Merge: cannot merge type %q", messageType)
	}
}

// MergeAdjacentSameType folds each run of consecutive same-type messages into
// one merged message. Non-adjacent runs and runs of size 1 are passed through
// unchanged.
func (l MessageList) MergeAdjacentSameType() MessageList {
	source := l.withoutNil()
	if len(source) <= 1 {
		return source
	}

	result := make(MessageList, 0, len(source))
	groupStart := 0
	for i := 1; i <= len(source); i++ {
		if i < len(source) && source[i].Type() == source[groupStart].Type() {
			continue
		}
		group := source[groupStart:i]
		if len(group) == 1 {
			result = append(result, group[0])
		} else if merged, err := group.Merge(group[0].Type()); err == nil {
			result = append(result, merged)
		} else {
			result = append(result, group...)
		}
		groupStart = i
	}
	return result
}

func (l MessageList) lastOfType(targetType MessageType) (int, Message) {
	for i := len(l) - 1; i >= 0; i-- {
		msg := l[i]
		if msg != nil && msg.Type() == targetType {
			return i, msg
		}
	}
	return -1, nil
}

func (l MessageList) augmentLastOfType(targetType MessageType, augmentFunc func(message Message) Message) {
	if augmentFunc == nil {
		return
	}

	idx, last := l.lastOfType(targetType)
	if idx == -1 {
		return
	}

	if augmented := augmentFunc(last); augmented != nil {
		l[idx] = augmented
	}
}

func (l MessageList) appendTextToLastOfType(targetType MessageType, text string) {
	l.augmentLastOfType(targetType, func(msg Message) Message {
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

func (l MessageList) replaceTextOfLastOfType(targetType MessageType, text string) {
	l.augmentLastOfType(targetType, func(msg Message) Message {
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

func (u *UserMessage) Transcript() string {
	return transcript(MessageTypeUser, u.Text)
}

func (s *SystemMessage) Transcript() string {
	return transcript(MessageTypeSystem, s.Text)
}

func (a *AssistantMessage) Transcript() string {
	var b strings.Builder
	b.WriteString(MessageTypeAssistant.String())
	b.WriteString(": ")
	b.WriteString(a.JoinedText())
	if a.HasToolCalls() {
		b.WriteByte('\n')
		calls := a.CollectToolCalls()
		data, _ := json.Marshal(calls)
		b.Write(data)
	}
	return b.String()
}

func (t *ToolMessage) Transcript() string {
	returns, _ := json.Marshal(t.ToolReturns)
	return transcript(MessageTypeTool, string(returns))
}

func transcript(messageType MessageType, payload string) string {
	return messageType.String() + ": " + payload
}

func (l MessageList) Strings() []string {
	out := make([]string, 0, len(l))
	for _, msg := range l {
		out = append(out, msg.Transcript())
	}
	return out
}
