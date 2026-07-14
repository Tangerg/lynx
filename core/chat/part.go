package chat

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/media"
)

// PartKind identifies which payload in Part is active.
type PartKind string

const (
	// PartText carries plain text.
	PartText PartKind = "text"
	// PartMedia carries an image, audio, document, or other media value.
	PartMedia PartKind = "media"
	// PartReasoning carries visible reasoning and an optional opaque signature.
	PartReasoning PartKind = "reasoning"
	// PartToolCall carries one tool invocation request.
	PartToolCall PartKind = "tool_call"
	// PartToolResult carries one tool execution result.
	PartToolResult PartKind = "tool_result"
)

// Valid reports whether k is a part kind known by the protocol.
func (k PartKind) Valid() bool {
	switch k {
	case PartText, PartMedia, PartReasoning, PartToolCall, PartToolResult:
		return true
	default:
		return false
	}
}

// Part is a tagged protocol value. Kind selects exactly one payload shape:
// Text, Media, reasoning Text/Signature, ToolCall, or ToolResult.
type Part struct {
	Kind       PartKind     `json:"kind"`
	Text       string       `json:"text,omitempty"`
	Media      *media.Media `json:"media,omitempty"`
	Signature  []byte       `json:"signature,omitempty"`
	ToolCall   *ToolCall    `json:"tool_call,omitempty"`
	ToolResult *ToolResult  `json:"tool_result,omitempty"`
}

// NewTextPart returns a text part.
func NewTextPart(text string) Part {
	return Part{Kind: PartText, Text: text}
}

// NewMediaPart returns a media part.
func NewMediaPart(value *media.Media) Part {
	return Part{Kind: PartMedia, Media: value}
}

// NewReasoningPart returns a reasoning part and copies signature.
func NewReasoningPart(text string, signature []byte) Part {
	return Part{Kind: PartReasoning, Text: text, Signature: slices.Clone(signature)}
}

// NewToolCallPart returns a tool-call part.
func NewToolCallPart(call ToolCall) Part {
	clone := call
	return Part{Kind: PartToolCall, ToolCall: &clone}
}

// NewToolResultPart returns a tool-result part.
func NewToolResultPart(result ToolResult) Part {
	clone := result
	return Part{Kind: PartToolResult, ToolResult: &clone}
}

// Validate verifies the discriminator, payload exclusivity, and active nested
// value.
func (p Part) Validate() error {
	if !p.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidPart, p.Kind)
	}

	switch p.Kind {
	case PartText:
		if p.Text == "" || p.Media != nil || len(p.Signature) != 0 || p.ToolCall != nil || p.ToolResult != nil {
			return fmt.Errorf("%w: kind %q requires non-empty text and no other payload", ErrInvalidPart, p.Kind)
		}
	case PartMedia:
		if p.Text != "" || p.Media == nil || len(p.Signature) != 0 || p.ToolCall != nil || p.ToolResult != nil {
			return fmt.Errorf("%w: kind %q requires media and no other payload", ErrInvalidPart, p.Kind)
		}
		if err := p.Media.Validate(); err != nil {
			return fmt.Errorf("%w: media: %w", ErrInvalidPart, err)
		}
	case PartReasoning:
		if (p.Text == "" && len(p.Signature) == 0) || p.Media != nil || p.ToolCall != nil || p.ToolResult != nil {
			return fmt.Errorf("%w: kind %q requires text or signature and no other payload", ErrInvalidPart, p.Kind)
		}
	case PartToolCall:
		if p.Text != "" || p.Media != nil || len(p.Signature) != 0 || p.ToolCall == nil || p.ToolResult != nil {
			return fmt.Errorf("%w: kind %q requires a tool call and no other payload", ErrInvalidPart, p.Kind)
		}
		if err := p.ToolCall.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPart, err)
		}
	case PartToolResult:
		if p.Text != "" || p.Media != nil || len(p.Signature) != 0 || p.ToolCall != nil || p.ToolResult == nil {
			return fmt.Errorf("%w: kind %q requires a tool result and no other payload", ErrInvalidPart, p.Kind)
		}
		if err := p.ToolResult.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPart, err)
		}
	}
	return nil
}

// MarshalJSON validates p before writing its tagged wire representation.
func (p Part) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	type wirePart Part
	return json.Marshal(wirePart(p))
}

// UnmarshalJSON decodes and validates a part before replacing the receiver.
func (p *Part) UnmarshalJSON(data []byte) error {
	if p == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidPart)
	}
	type wirePart Part
	var decoded wirePart
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidPart, err)
	}
	candidate := Part(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*p = candidate
	return nil
}
