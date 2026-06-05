package chat

import (
	"errors"
	"fmt"
	"iter"
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
