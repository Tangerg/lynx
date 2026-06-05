package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
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

	// MessageTypeAssistant is one model reply — text / reasoning / tool-call
	// requests, all carried as ordered [OutputPart]s.
	MessageTypeAssistant MessageType = "assistant"

	// MessageTypeTool carries the results of executing tool calls the
	// assistant requested in the previous turn.
	MessageTypeTool MessageType = "tool"
)

func (t MessageType) String() string { return string(t) }

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

// ToolReturn is the result of executing one tool call, correlated by ID
// back to a [ToolCallPart] from the previous assistant turn.
type ToolReturn struct {
	// ID matches the originating ToolCallPart.ID.
	ID string `json:"id"`

	// Name is the executed tool's name (mirrors ToolCallPart.Name).
	Name string `json:"name"`

	// Result is the tool's textual output.
	Result string `json:"result"`

	// Artifact is an optional typed value a tool can attach alongside its
	// textual Result (see [ArtifactTool]). It is NEVER sent to the model or
	// serialized into chat history (json:"-") — it rides the tool message
	// for non-LLM consumers (e.g. an agent action sinking it onto the
	// blackboard). Nil for ordinary tools.
	Artifact any `json:"-"`
}

// MessageParams is the universal constructor input — the message-type
// constructors below pick the fields they care about. It is ALSO the
// canonical wire shape: every [Message] implementation marshals to and
// unmarshals from MessageParams JSON, with [MessageParams.Type] acting
// as the discriminator. Use it directly when you need full control;
// otherwise the typed shortcuts (string, []*media.Media, ...) are
// usually enough.
type MessageParams struct {
	// Type selects which message constructor [NewMessage] dispatches to,
	// and discriminates the role on the wire.
	Type MessageType `json:"type"`

	// Text is the textual body. Used by system / user messages; for
	// assistant messages it is converted to a single [TextPart].
	Text string `json:"text,omitempty"`

	// Parts is the ordered content list — used by assistant messages.
	// When both Text and Parts are supplied for an assistant message,
	// Text is appended after Parts as a trailing TextPart.
	Parts []OutputPart `json:"parts,omitzero"`

	// Metadata holds arbitrary per-message extras.
	Metadata map[string]any `json:"metadata,omitzero"`

	// Media holds attachments (images, documents, audio) — used by user
	// messages.
	Media []*media.Media `json:"media,omitzero"`

	// ToolReturns are tool execution results on tool messages.
	ToolReturns []*ToolReturn `json:"tool_returns,omitzero"`
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

// AssistantMessage is one model reply, carried as an ordered list of
// [OutputPart]s. Text, reasoning, and tool calls live in [Parts] in
// the order the model emitted them — text↔tool_use interleaving from
// Claude / Gemini / OpenAI Responses API is preserved verbatim.
//
// Helper accessors ([AssistantMessage.JoinedText],
// [AssistantMessage.JoinedReasoning], [AssistantMessage.ToolCalls],
// ...) derive flat views from Parts for code that just wants the
// final string or tool-call list.
type AssistantMessage struct {
	Parts    []OutputPart   `json:"parts"`
	Metadata map[string]any `json:"metadata,omitzero"`
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

// TextParts iterates the [TextPart]s in this message, in order.
func (a *AssistantMessage) TextParts() iter.Seq[*TextPart] {
	return partsOf[*TextPart](a)
}

// ReasoningParts iterates the [ReasoningPart]s in this message, in order.
func (a *AssistantMessage) ReasoningParts() iter.Seq[*ReasoningPart] {
	return partsOf[*ReasoningPart](a)
}

// ToolCalls iterates the [ToolCallPart]s in this message, in order.
func (a *AssistantMessage) ToolCalls() iter.Seq[*ToolCallPart] {
	return partsOf[*ToolCallPart](a)
}

// partsOf yields the parts assignable to T, in order. Nil-receiver safe.
func partsOf[T OutputPart](a *AssistantMessage) iter.Seq[T] {
	return func(yield func(T) bool) {
		if a == nil {
			return
		}
		for _, p := range a.Parts {
			if tp, ok := p.(T); ok && !yield(tp) {
				return
			}
		}
	}
}

// CollectToolCalls returns the [ToolCallPart]s as a slice — the
// allocating counterpart of [AssistantMessage.ToolCalls] for sites
// that need indexed access or len().
func (a *AssistantMessage) CollectToolCalls() []*ToolCallPart {
	return slices.Collect(a.ToolCalls())
}

// JoinedText concatenates the text bodies of every [TextPart] (no
// separator). Use when downstream just needs "the final string the
// user sees".
func (a *AssistantMessage) JoinedText() string {
	return joinTexts(a.TextParts(), func(p *TextPart) string { return p.Text })
}

// JoinedReasoning concatenates the text bodies of every
// [ReasoningPart] (no separator).
func (a *AssistantMessage) JoinedReasoning() string {
	return joinTexts(a.ReasoningParts(), func(p *ReasoningPart) string { return p.Text })
}

// joinTexts concatenates each part's text body (extracted by getText)
// into a single string without separators.
func joinTexts[T any](seq iter.Seq[T], getText func(T) string) string {
	var b strings.Builder
	for p := range seq {
		b.WriteString(getText(p))
	}
	return b.String()
}

// HasToolCalls reports whether any [ToolCallPart] is present.
func (a *AssistantMessage) HasToolCalls() bool {
	if a == nil {
		return false
	}
	return slices.ContainsFunc(a.Parts, func(p OutputPart) bool {
		_, ok := p.(*ToolCallPart)
		return ok
	})
}

// HasReasoning reports whether the message carries any non-empty reasoning text.
func (a *AssistantMessage) HasReasoning() bool {
	if a == nil {
		return false
	}
	return slices.ContainsFunc(a.Parts, func(p OutputPart) bool {
		rp, ok := p.(*ReasoningPart)
		return ok && rp.Text != ""
	})
}

// NewAssistantMessage builds an [AssistantMessage] from one of the
// supported parameter shapes — the type-set on T documents the
// accepted forms:
//
//   - string                → single [TextPart]
//   - []OutputPart          → use Parts verbatim
//   - []*ToolCallPart       → use as Parts (one per call)
//   - map[string]any        → metadata only (empty Parts)
//   - [MessageParams]       → full control
func NewAssistantMessage[T string | []OutputPart | []*ToolCallPart | map[string]any | MessageParams](param T) *AssistantMessage {
	params := paramsFromAssistantInput(param)

	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}

	parts := params.Parts
	// MessageParams.Text — when supplied alongside Parts, gets emitted
	// as a trailing TextPart. When the only input is a string, the
	// switch in paramsFromAssistantInput already set Parts directly.
	if params.Text != "" && !textAlreadyInParts(parts, params.Text) {
		parts = append(parts, &TextPart{Text: params.Text})
	}

	return &AssistantMessage{
		Parts:    parts,
		Metadata: params.Metadata,
	}
}

// paramsFromAssistantInput unpacks the polymorphic input into a single
// MessageParams so NewAssistantMessage can stay focused on field setup.
func paramsFromAssistantInput[T string | []OutputPart | []*ToolCallPart | map[string]any | MessageParams](param T) MessageParams {
	var out MessageParams
	switch typed := any(param).(type) {
	case string:
		if typed != "" {
			out.Parts = []OutputPart{&TextPart{Text: typed}}
		}
	case []OutputPart:
		out.Parts = typed
	case []*ToolCallPart:
		out.Parts = pkgSlices.Map(typed, func(tc *ToolCallPart) OutputPart { return tc })
	case map[string]any:
		out.Metadata = typed
	case MessageParams:
		out = typed
	}
	return out
}

// textAlreadyInParts guards against double-appending Text when both
// Text and Parts are passed via MessageParams and Parts ends with the
// same string.
func textAlreadyInParts(parts []OutputPart, text string) bool {
	last, ok := pkgSlices.Last(parts)
	if !ok {
		return false
	}
	tp, isText := last.(*TextPart)
	return isText && tp.Text == text
}

// SystemMessage shapes the model's behavior for the whole conversation —
// persona, response format, guardrails. It typically sits at the head
// of the message list.
type SystemMessage struct {
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitzero"`
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
	return &SystemMessage{
		Text:     params.Text,
		Metadata: params.Metadata,
	}
}

// UserMessage is one user turn — text, optional media, optional metadata.
type UserMessage struct {
	Text     string         `json:"text"`
	Media    []*media.Media `json:"media,omitzero"`
	Metadata map[string]any `json:"metadata,omitzero"`
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
func (u *UserMessage) HasMedia() bool { return u != nil && len(u.Media) > 0 }

// NewUserMessage builds a [UserMessage] from a raw text string, media
// slice, or full [MessageParams].
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
	return &UserMessage{
		Text:     params.Text,
		Media:    params.Media,
		Metadata: params.Metadata,
	}
}

// ToolMessage carries the results of executing tool calls the assistant
// requested in the previous turn.
type ToolMessage struct {
	ToolReturns []*ToolReturn  `json:"tool_returns,omitzero"`
	Metadata    map[string]any `json:"metadata,omitzero"`
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

// MarshalJSON encodes the message in the canonical [MessageParams]
// shape.
func (s *SystemMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(MessageParams{
		Type:     MessageTypeSystem,
		Text:     s.Text,
		Metadata: s.Metadata,
	})
}

// UnmarshalJSON decodes from the [MessageParams] wire shape.
func (s *SystemMessage) UnmarshalJSON(data []byte) error {
	var p MessageParams
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Text = p.Text
	s.Metadata = p.Metadata
	return nil
}

// MarshalJSON encodes the message in the canonical [MessageParams]
// shape.
func (u *UserMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(MessageParams{
		Type:     MessageTypeUser,
		Text:     u.Text,
		Media:    u.Media,
		Metadata: u.Metadata,
	})
}

// UnmarshalJSON decodes from the [MessageParams] wire shape.
func (u *UserMessage) UnmarshalJSON(data []byte) error {
	var p MessageParams
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	u.Text = p.Text
	u.Media = p.Media
	u.Metadata = p.Metadata
	return nil
}

// MarshalJSON encodes the assistant message as a kind-tagged Parts
// array, preserving order verbatim. Each part renders as a flat JSON
// object with a "kind" discriminator so generic decoders can dispatch
// without per-message-type knowledge.
func (a *AssistantMessage) MarshalJSON() ([]byte, error) {
	parts := make([]json.RawMessage, 0, len(a.Parts))
	for _, p := range a.Parts {
		raw, err := marshalOutputPart(p)
		if err != nil {
			return nil, err
		}
		parts = append(parts, raw)
	}
	return json.Marshal(struct {
		Type     MessageType       `json:"type"`
		Parts    []json.RawMessage `json:"parts"`
		Metadata map[string]any    `json:"metadata,omitzero"`
	}{
		Type:     MessageTypeAssistant,
		Parts:    parts,
		Metadata: a.Metadata,
	})
}

// UnmarshalJSON decodes the kind-tagged Parts array back into a typed
// []OutputPart, preserving order.
func (a *AssistantMessage) UnmarshalJSON(data []byte) error {
	var w struct {
		Parts    []json.RawMessage `json:"parts"`
		Metadata map[string]any    `json:"metadata"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	parts := make([]OutputPart, 0, len(w.Parts))
	for _, raw := range w.Parts {
		p, err := unmarshalOutputPart(raw)
		if err != nil {
			return err
		}
		parts = append(parts, p)
	}
	a.Parts = parts
	a.Metadata = w.Metadata
	return nil
}

// MarshalJSON encodes the message in the canonical [MessageParams]
// shape.
func (t *ToolMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(MessageParams{
		Type:        MessageTypeTool,
		ToolReturns: t.ToolReturns,
		Metadata:    t.Metadata,
	})
}

// UnmarshalJSON decodes from the [MessageParams] wire shape.
func (t *ToolMessage) UnmarshalJSON(data []byte) error {
	var p MessageParams
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	t.ToolReturns = p.ToolReturns
	t.Metadata = p.Metadata
	return nil
}

// UnmarshalMessage decodes a JSON payload in the [MessageParams] wire
// shape into a concrete [Message]. The Type field acts as a
// discriminator. For assistant messages the kind-tagged Parts array
// is decoded into typed [OutputPart]s.
func UnmarshalMessage(data []byte) (Message, error) {
	// Discriminator pass: read type only, then dispatch to the typed
	// UnmarshalJSON of the concrete message.
	var head struct {
		Type MessageType `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	switch head.Type {
	case MessageTypeSystem:
		var m SystemMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		return &m, nil
	case MessageTypeUser:
		var m UserMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		return &m, nil
	case MessageTypeAssistant:
		var m AssistantMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		return &m, nil
	case MessageTypeTool:
		var m ToolMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		return &m, nil
	default:
		return nil, fmt.Errorf("chat.UnmarshalMessage: unsupported type %q", head.Type)
	}
}
