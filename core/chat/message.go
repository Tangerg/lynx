package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

var (
	// ErrInvalidMessage reports a malformed message or an invalid role/part
	// combination.
	ErrInvalidMessage = errors.New("chat: invalid message")
	// ErrInvalidPart reports a malformed or ambiguous tagged part.
	ErrInvalidPart = errors.New("chat: invalid part")
	// ErrInvalidToolCall reports a malformed tool invocation request.
	ErrInvalidToolCall = errors.New("chat: invalid tool call")
	// ErrInvalidToolResult reports a malformed tool execution result.
	ErrInvalidToolResult = errors.New("chat: invalid tool result")
)

// Role identifies a message's participant in a conversation.
type Role string

const (
	// RoleSystem carries model instructions.
	RoleSystem Role = "system"
	// RoleUser carries user input.
	RoleUser Role = "user"
	// RoleAssistant carries model output.
	RoleAssistant Role = "assistant"
	// RoleTool carries results for tool calls requested by the assistant.
	RoleTool Role = "tool"
)

// Valid reports whether r is a role known by the protocol.
func (r Role) Valid() bool {
	switch r {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

// Message is one provider-neutral conversation entry. Parts retain their
// order so interleaved assistant text, reasoning, and tool calls round-trip.
type Message struct {
	Role     Role         `json:"role"`
	Parts    []Part       `json:"parts"`
	Metadata metadata.Map `json:"metadata,omitempty"`
}

// NewSystemMessage returns a system message containing text.
func NewSystemMessage(text string) Message {
	return Message{Role: RoleSystem, Parts: []Part{NewTextPart(text)}}
}

// NewUserMessage returns a user message containing parts in their supplied
// order.
func NewUserMessage(parts ...Part) Message {
	return Message{Role: RoleUser, Parts: slices.Clone(parts)}
}

// NewAssistantMessage returns an assistant message containing parts in their
// supplied order.
func NewAssistantMessage(parts ...Part) Message {
	return Message{Role: RoleAssistant, Parts: slices.Clone(parts)}
}

// NewToolMessage returns a tool message containing one part per result.
func NewToolMessage(results ...ToolResult) Message {
	parts := make([]Part, len(results))
	for i := range results {
		parts[i] = NewToolResultPart(results[i])
	}
	return Message{Role: RoleTool, Parts: parts}
}

// Text concatenates text parts in order. It is nil-safe and ignores media,
// reasoning, and tool parts.
func (m *Message) Text() string {
	if m == nil {
		return ""
	}
	var text strings.Builder
	for i := range m.Parts {
		if m.Parts[i].Kind == PartText {
			text.WriteString(m.Parts[i].Text)
		}
	}
	return text.String()
}

// Validate verifies the role, every nested protocol value, metadata, and the
// role/part compatibility matrix.
func (m Message) Validate() error {
	if !m.Role.Valid() {
		return fmt.Errorf("%w: unknown role %q", ErrInvalidMessage, m.Role)
	}
	if len(m.Parts) == 0 {
		return fmt.Errorf("%w: role %q requires at least one part", ErrInvalidMessage, m.Role)
	}
	if err := m.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidMessage, err)
	}
	for i := range m.Parts {
		part := m.Parts[i]
		if err := part.Validate(); err != nil {
			return fmt.Errorf("%w: parts[%d]: %w", ErrInvalidMessage, i, err)
		}
		if !roleAllowsPart(m.Role, part.Kind) {
			return fmt.Errorf("%w: role %q does not allow part kind %q", ErrInvalidMessage, m.Role, part.Kind)
		}
	}
	return nil
}

func roleAllowsPart(role Role, kind PartKind) bool {
	switch role {
	case RoleSystem:
		return kind == PartText
	case RoleUser:
		return kind == PartText || kind == PartMedia
	case RoleAssistant:
		return kind == PartText || kind == PartMedia || kind == PartReasoning || kind == PartToolCall
	case RoleTool:
		return kind == PartToolResult
	default:
		return false
	}
}

// MarshalJSON validates m before writing its tagged wire representation.
func (m Message) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	type wireMessage Message
	return json.Marshal(wireMessage(m))
}

// UnmarshalJSON decodes and validates a message before replacing the receiver.
func (m *Message) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidMessage)
	}
	type wireMessage Message
	var decoded wireMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidMessage, err)
	}
	candidate := Message(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
}
