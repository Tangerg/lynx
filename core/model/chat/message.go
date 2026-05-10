package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/media"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// MessageType is the role tag attached to every chat message — provider
// APIs use it to route content (system prompts, user turns, assistant
// replies, tool results).
type MessageType string

const (
	// MessageTypeSystem is high-level instructions that shape the model's
	// behavior for the whole conversation (persona, response format, ...).
	MessageTypeSystem MessageType = "system"

	// MessageTypeUser is one user turn — a question, prompt, or input.
	MessageTypeUser MessageType = "user"

	// MessageTypeAssistant is one model reply — text, optional media, and
	// optional tool-call requests.
	MessageTypeAssistant MessageType = "assistant"

	// MessageTypeTool carries the results of executing tool calls the
	// assistant requested in the previous turn.
	MessageTypeTool MessageType = "tool"
)

func (t MessageType) String() string  { return string(t) }
func (t MessageType) IsSystem() bool  { return t == MessageTypeSystem }
func (t MessageType) IsUser() bool    { return t == MessageTypeUser }
func (t MessageType) IsAssistant() bool { return t == MessageTypeAssistant }
func (t MessageType) IsTool() bool    { return t == MessageTypeTool }

// Message is the sealed interface implemented by [SystemMessage],
// [UserMessage], [AssistantMessage], and [ToolMessage]. Concrete types
// satisfy it via the unexported message() method, so external packages
// cannot introduce new message types — keeping the planner's switch
// statements exhaustive.
type Message interface {
	// Type returns the role this message plays in the conversation.
	Type() MessageType

	// Meta returns the metadata map; it is allocated lazily so callers do
	// not have to nil-check before reading or writing.
	Meta() map[string]any

	// message keeps the interface sealed.
	message()
}

// ToolCall is one function-call request the assistant emitted. The
// JSON-encoded Arguments are passed through verbatim — the tool runtime
// is responsible for parsing them against the tool's input schema.
type ToolCall struct {
	// ID uniquely identifies this call so a later ToolReturn can match.
	ID string `json:"id"`

	// Name selects which tool to invoke.
	Name string `json:"name"`

	// Arguments is the JSON-encoded argument object.
	Arguments string `json:"arguments"`
}

// ToolReturn is the result of executing one [ToolCall], correlated by ID.
type ToolReturn struct {
	// ID matches the originating ToolCall.ID.
	ID string `json:"id"`

	// Name is the executed tool's name (mirrors ToolCall.Name).
	Name string `json:"name"`

	// Result is the tool's textual output.
	Result string `json:"result"`
}

// MessageParams is the universal constructor input — the message-type
// constructors below pick the fields they care about. Use it directly
// when you need full control; otherwise the typed shortcuts (string,
// []*media.Media, ...) are usually enough.
type MessageParams struct {
	// Type selects which message constructor [NewMessage] dispatches to.
	Type MessageType `json:"type"`

	// Text is the textual body.
	Text string `json:"text"`

	// Reasoning is the visible chain-of-thought, populated only by
	// reasoning-style assistants.
	Reasoning string `json:"reasoning"`

	// Metadata holds arbitrary per-message extras.
	Metadata map[string]any `json:"metadata"`

	// Media holds attachments (images, documents, audio).
	Media []*media.Media `json:"media"`

	// ToolCalls are tool-invocation requests on assistant messages.
	ToolCalls []*ToolCall `json:"tool_calls"`

	// ToolReturns are tool execution results on tool messages.
	ToolReturns []*ToolReturn `json:"tool_returns"`
}

// NewMessage dispatches to the matching message-type constructor based
// on params.Type.
func NewMessage(params MessageParams) (Message, error) {
	switch params.Type {
	case MessageTypeSystem:
		return NewSystemMessage(params), nil
	case MessageTypeAssistant:
		return NewAssistantMessage(params), nil
	case MessageTypeUser:
		return NewUserMessage(params), nil
	case MessageTypeTool:
		return NewToolMessage(params)
	default:
		return nil, fmt.Errorf("chat.NewMessage: unsupported message type %q", params.Type)
	}
}

// AssistantMessage is one model reply.
//
// Reasoning carries the visible chain-of-thought from reasoning-style
// providers (Anthropic extended thinking, DeepSeek-R1 reasoning_content,
// Gemini thoughts). It is independent of [AssistantMessage.Text]:
// providers that expose reasoning publish it here; providers that do not
// leave it empty.
//
// Provider-specific continuation tokens (Anthropic signature, Google
// thoughtSignatures, redacted-thinking payloads) live in Metadata under
// well-known keys following the "lynx:chat:<provider>:<concept>"
// convention.
type AssistantMessage struct {
	Text      string         `json:"text"`
	Reasoning string         `json:"reasoning,omitempty"`
	Media     []*media.Media `json:"media"`
	ToolCalls []*ToolCall    `json:"tool_calls"`
	Metadata  map[string]any `json:"metadata"`
}

func (a *AssistantMessage) message() {}

// Type reports MessageTypeAssistant.
func (a *AssistantMessage) Type() MessageType { return MessageTypeAssistant }

// Meta returns the metadata map, allocating it on first access.
func (a *AssistantMessage) Meta() map[string]any {
	if a.Metadata == nil {
		a.Metadata = make(map[string]any)
	}
	return a.Metadata
}

// HasMedia reports whether the message carries any attachments.
func (a *AssistantMessage) HasMedia() bool { return len(a.Media) > 0 }

// HasToolCalls reports whether the model requested any tool invocations.
func (a *AssistantMessage) HasToolCalls() bool { return len(a.ToolCalls) > 0 }

// HasReasoning reports whether the message carries visible
// chain-of-thought text. Returns false for nil receivers.
func (a *AssistantMessage) HasReasoning() bool { return a != nil && a.Reasoning != "" }

// NewAssistantMessage builds an [AssistantMessage] from one of the
// supported parameter shapes — the type-set on T documents the
// accepted forms (text, media, tool calls, metadata, or full params).
func NewAssistantMessage[T string | []*media.Media | []*ToolCall | map[string]any | MessageParams](param T) *AssistantMessage {
	params := paramsFromAssistantInput(param)

	if params.Media == nil {
		params.Media = make([]*media.Media, 0)
	}
	if params.ToolCalls == nil {
		params.ToolCalls = make([]*ToolCall, 0)
	}
	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}

	return &AssistantMessage{
		Text:      params.Text,
		Reasoning: params.Reasoning,
		Media:     params.Media,
		ToolCalls: params.ToolCalls,
		Metadata:  params.Metadata,
	}
}

// paramsFromAssistantInput unpacks the polymorphic input into a single
// MessageParams so NewAssistantMessage can stay focused on field setup.
func paramsFromAssistantInput[T string | []*media.Media | []*ToolCall | map[string]any | MessageParams](param T) MessageParams {
	var out MessageParams
	switch typed := any(param).(type) {
	case string:
		out.Text = typed
	case []*media.Media:
		out.Media = typed
	case []*ToolCall:
		out.ToolCalls = typed
	case map[string]any:
		out.Metadata = typed
	case MessageParams:
		out = typed
	}
	return out
}

// SystemMessage shapes the model's behavior for the whole conversation —
// persona, response format, guardrails. It typically sits at the head
// of the message list.
type SystemMessage struct {
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata"`
}

func (s *SystemMessage) message() {}

// Type reports MessageTypeSystem.
func (s *SystemMessage) Type() MessageType { return MessageTypeSystem }

// Meta returns the metadata map, allocating it on first access.
func (s *SystemMessage) Meta() map[string]any {
	if s.Metadata == nil {
		s.Metadata = make(map[string]any)
	}
	return s.Metadata
}

// NewSystemMessage builds a [SystemMessage] from a raw text string or
// full [MessageParams].
func NewSystemMessage[T string | MessageParams](param T) *SystemMessage {
	var params MessageParams
	switch typed := any(param).(type) {
	case string:
		params.Text = typed
	case MessageParams:
		params = typed
	}

	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}

	return &SystemMessage{
		Text:     params.Text,
		Metadata: params.Metadata,
	}
}

// UserMessage is one user turn — text, optional media, optional metadata.
type UserMessage struct {
	Text     string         `json:"text"`
	Media    []*media.Media `json:"media"`
	Metadata map[string]any `json:"metadata"`
}

func (u *UserMessage) message() {}

// Type reports MessageTypeUser.
func (u *UserMessage) Type() MessageType { return MessageTypeUser }

// Meta returns the metadata map, allocating it on first access.
func (u *UserMessage) Meta() map[string]any {
	if u.Metadata == nil {
		u.Metadata = make(map[string]any)
	}
	return u.Metadata
}

// HasMedia reports whether any attachments are present.
func (u *UserMessage) HasMedia() bool { return len(u.Media) > 0 }

// NewUserMessage builds a [UserMessage] from a raw text string,
// media slice, or full [MessageParams].
func NewUserMessage[T string | []*media.Media | MessageParams](param T) *UserMessage {
	var params MessageParams
	switch typed := any(param).(type) {
	case string:
		params.Text = typed
	case []*media.Media:
		params.Media = typed
	case MessageParams:
		params = typed
	}

	if params.Media == nil {
		params.Media = make([]*media.Media, 0)
	}
	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}

	return &UserMessage{
		Text:     params.Text,
		Media:    params.Media,
		Metadata: params.Metadata,
	}
}

// ToolMessage carries the results of executing tool calls the assistant
// requested in the previous turn.
type ToolMessage struct {
	ToolReturns []*ToolReturn  `json:"tool_returns"`
	Metadata    map[string]any `json:"metadata"`
}

func (t *ToolMessage) message() {}

// Type reports MessageTypeTool.
func (t *ToolMessage) Type() MessageType { return MessageTypeTool }

// Meta returns the metadata map, allocating it on first access.
func (t *ToolMessage) Meta() map[string]any {
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	return t.Metadata
}

// NewToolMessage builds a [ToolMessage] from a tool-return slice or
// full [MessageParams]. Returns an error when no tool returns are
// supplied — a tool message with no results is meaningless.
func NewToolMessage[T []*ToolReturn | MessageParams](param T) (*ToolMessage, error) {
	var params MessageParams
	switch typed := any(param).(type) {
	case []*ToolReturn:
		params.ToolReturns = typed
	case MessageParams:
		params = typed
	}

	if len(params.ToolReturns) == 0 {
		return nil, errors.New("chat.NewToolMessage: at least one ToolReturn is required")
	}

	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}

	return &ToolMessage{
		ToolReturns: params.ToolReturns,
		Metadata:    params.Metadata,
	}, nil
}

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
		return make([]Message, 0)
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
//
// Example:
//
//	users := chat.FilterMessagesByMessageTypes(history, chat.MessageTypeUser)
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
// Text concatenates with double-newline separators, media concatenates,
// metadata merges last-write-wins. Returns nil when no user message
// exists.
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
	mergedMedia := make([]*media.Media, 0)

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

// MergeToolMessages collapses every [ToolMessage] in messages into one,
// concatenating tool returns and merging metadata last-write-wins.
// Returns (nil, nil) when no tool message exists.
func MergeToolMessages(messages []Message) (*ToolMessage, error) {
	tools := FilterMessagesByMessageTypes(messages, MessageTypeTool)

	if len(tools) == 0 {
		return nil, nil
	}
	if len(tools) == 1 {
		return tools[0].(*ToolMessage), nil
	}

	merged := make(map[string]any)
	mergedReturns := make([]*ToolReturn, 0)

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
// messages cannot be merged — each represents a distinct model turn —
// and that case returns an error.
func MergeMessages(messages []Message, messageType MessageType) (Message, error) {
	switch {
	case messageType.IsSystem():
		return MergeSystemMessages(messages), nil
	case messageType.IsUser():
		return MergeUserMessages(messages), nil
	case messageType.IsTool():
		return MergeToolMessages(messages)
	default:
		return nil, fmt.Errorf("chat.MergeMessages: cannot merge type %q", messageType)
	}
}

// MergeAdjacentSameTypeMessages folds each run of consecutive same-type
// messages into one merged message. Non-adjacent runs and runs of size 1
// are passed through unchanged. Nil entries are filtered out first.
//
// Example:
//
//	in:  [user, user, system, user, tool, tool]
//	out: [merged-user, system, user, merged-tool]
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
			// Merging failed (typically: assistant runs aren't mergeable) —
			// keep the originals so no information is lost.
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
// runs augmentFunc on it in place. Nothing happens when augmentFunc is
// nil or no matching message exists.
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
// ignored — the assistant and tool roles do not own a single editable
// text channel.
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
// system message. Other types are silently ignored, matching
// [appendTextToLastMessageOfType].
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

// MessageToString renders one message as "role: payload". Tool calls and
// tool returns serialize as compact JSON; other content is emitted
// verbatim.
//
// Example:
//
//	chat.MessageToString(chat.NewUserMessage("hi"))
//	// "user: hi"
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
		b.WriteString(typed.Text)
		if typed.HasToolCalls() {
			b.WriteByte('\n')
			calls, _ := json.Marshal(typed.ToolCalls)
			b.Write(calls)
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
