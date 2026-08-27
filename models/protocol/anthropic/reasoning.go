package anthropic

import (
	"errors"
	"fmt"

	corechat "github.com/Tangerg/scope/core/chat"
)

// ReasoningBlockKind identifies the official Anthropic reasoning block carried
// by a Core reasoning part.
type ReasoningBlockKind string

const (
	ReasoningBlockThinking ReasoningBlockKind = protocolReasoningThinking
	ReasoningBlockRedacted ReasoningBlockKind = protocolReasoningRedacted
)

func NewThinkingPart(text string, signature []byte) (corechat.Part, error) {
	if len(signature) == 0 {
		return corechat.Part{}, errors.New("anthropic: thinking signature is required")
	}
	part := corechat.NewReasoningPart(text, signature)
	if err := setProtocolReasoningState(&part, protocolProvider, protocolReasoningThinking); err != nil {
		return corechat.Part{}, err
	}
	return part, nil
}

func NewRedactedThinkingPart(data []byte) (corechat.Part, error) {
	if len(data) == 0 {
		return corechat.Part{}, errors.New("anthropic: redacted thinking data is required")
	}
	part := corechat.NewReasoningPart("", data)
	if err := setProtocolReasoningState(&part, protocolProvider, protocolReasoningRedacted); err != nil {
		return corechat.Part{}, err
	}
	return part, nil
}

// ReasoningBlockKindOf reports whether part contains Anthropic-issued
// reasoning replay state and, when it does, which official block variant it is.
func ReasoningBlockKindOf(part corechat.Part) (ReasoningBlockKind, bool, error) {
	value, found, err := part.Metadata.Decode[string](protocolReasoningKindKey)
	if err != nil {
		return "", found, fmt.Errorf("anthropic: reasoning metadata: %w", err)
	}
	if !found {
		return "", false, nil
	}
	kind := ReasoningBlockKind(value)
	switch kind {
	case ReasoningBlockThinking, ReasoningBlockRedacted:
		return kind, true, nil
	default:
		return "", true, fmt.Errorf("anthropic: unknown reasoning block kind %q", value)
	}
}
