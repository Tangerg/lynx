package chat

import (
	"fmt"

	"github.com/Tangerg/lynx/core/media"
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

	// Transcript renders the message as a role-prefixed text projection.
	Transcript() string

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
