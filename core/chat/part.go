package chat

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
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
	// PartToolCallDelta carries one streaming tool-call fragment. It is response-
	// only and [ResponseAccumulator] promotes it into PartToolCall.
	PartToolCallDelta PartKind = "tool_call_delta"
	// PartToolResult carries one tool execution result.
	PartToolResult PartKind = "tool_result"
)

func (p PartKind) Valid() bool {
	switch p {
	case PartText, PartMedia, PartReasoning, PartToolCall, PartToolCallDelta, PartToolResult:
		return true
	default:
		return false
	}
}

// Part is a tagged protocol value. Kind selects exactly one payload shape:
// Text, Media, reasoning Text/Signature, ToolCall, or ToolResult. Metadata
// retains JSON-safe, part-scoped provider state without weakening the common
// semantic payload.
type Part struct {
	Kind          PartKind       `json:"kind"`
	Text          string         `json:"text,omitempty"`
	Media         *media.Media   `json:"media,omitempty"`
	Signature     []byte         `json:"signature,omitempty"`
	ToolCall      *ToolCall      `json:"tool_call,omitempty"`
	ToolCallDelta *ToolCallDelta `json:"tool_call_delta,omitempty"`
	ToolResult    *ToolResult    `json:"tool_result,omitempty"`
	Metadata      metadata.Map   `json:"metadata,omitzero"`
}

type partPayload uint8

const (
	payloadText partPayload = 1 << iota
	payloadMedia
	payloadSignature
	payloadToolCall
	payloadToolCallDelta
	payloadToolResult
)

func (p Part) Clone() Part {
	clone := p
	clone.Signature = slices.Clone(p.Signature)
	clone.Media = p.Media.Clone()
	clone.Metadata = p.Metadata.Clone()
	if p.ToolCall != nil {
		clone.ToolCall = new(*p.ToolCall)
	}
	if p.ToolCallDelta != nil {
		clone.ToolCallDelta = new(*p.ToolCallDelta)
	}
	if p.ToolResult != nil {
		result := p.ToolResult.Clone()
		clone.ToolResult = &result
	}
	return clone
}

func NewTextPart(text string) Part {
	return Part{Kind: PartText, Text: text}
}

func NewMediaPart(value *media.Media) Part {
	return Part{Kind: PartMedia, Media: value}
}

func NewReasoningPart(text string, signature []byte) Part {
	return Part{Kind: PartReasoning, Text: text, Signature: slices.Clone(signature)}
}

func NewToolCallPart(call ToolCall) Part {
	return Part{Kind: PartToolCall, ToolCall: new(call)}
}

func NewToolCallDeltaPart(delta ToolCallDelta) Part {
	return Part{Kind: PartToolCallDelta, ToolCallDelta: new(delta)}
}

func NewToolResultPart(result ToolResult) Part {
	return Part{Kind: PartToolResult, ToolResult: new(result)}
}

func (p Part) Validate() error {
	if !p.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidPart, p.Kind)
	}
	if err := p.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidPart, err)
	}

	payload := p.payload()
	switch p.Kind {
	case PartText:
		return p.validateTextPayload(payload)
	case PartMedia:
		return p.validateMediaPayload(payload)
	case PartReasoning:
		return p.validateReasoningPayload(payload)
	case PartToolCall:
		return validatePartPayload(p.Kind, payload, payloadToolCall, func() error { return p.ToolCall.Validate() })
	case PartToolCallDelta:
		return validatePartPayload(p.Kind, payload, payloadToolCallDelta, func() error { return p.ToolCallDelta.Validate() })
	case PartToolResult:
		return validatePartPayload(p.Kind, payload, payloadToolResult, func() error { return p.ToolResult.Validate() })
	}
	return nil
}

func (p Part) payload() partPayload {
	var payload partPayload
	if p.Text != "" {
		payload |= payloadText
	}
	if p.Media != nil {
		payload |= payloadMedia
	}
	if len(p.Signature) != 0 {
		payload |= payloadSignature
	}
	if p.ToolCall != nil {
		payload |= payloadToolCall
	}
	if p.ToolCallDelta != nil {
		payload |= payloadToolCallDelta
	}
	if p.ToolResult != nil {
		payload |= payloadToolResult
	}
	return payload
}

func (p Part) validateTextPayload(payload partPayload) error {
	if payload == payloadText || payload == 0 && len(p.Metadata) > 0 {
		return nil
	}
	return fmt.Errorf("%w: kind %q requires text or metadata and no other payload", ErrInvalidPart, p.Kind)
}

func (p Part) validateMediaPayload(payload partPayload) error {
	if payload != payloadMedia {
		return fmt.Errorf("%w: kind %q requires media and no other payload", ErrInvalidPart, p.Kind)
	}
	if err := p.Media.Validate(); err != nil {
		return fmt.Errorf("%w: media: %w", ErrInvalidPart, err)
	}
	return nil
}

func (p Part) validateReasoningPayload(payload partPayload) error {
	const allowed = payloadText | payloadSignature
	if payload == 0 || payload&^allowed != 0 {
		return fmt.Errorf("%w: kind %q requires text or signature and no other payload", ErrInvalidPart, p.Kind)
	}
	return nil
}

func validatePartPayload(kind PartKind, actual, required partPayload, validate func() error) error {
	if actual != required {
		return fmt.Errorf("%w: kind %q requires its matching payload and no other payload", ErrInvalidPart, kind)
	}
	if err := validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPart, err)
	}
	return nil
}

func (p Part) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	type wirePart Part
	return json.Marshal(wirePart(p))
}

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
