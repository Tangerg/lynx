package bedrock

import (
	"errors"
	"fmt"

	corechat "github.com/Tangerg/scope/core/chat"
)

// ReasoningBlockKind identifies a Bedrock Converse reasoning-content variant.
type ReasoningBlockKind string

const (
	ReasoningBlockText     ReasoningBlockKind = chatReasoningText
	ReasoningBlockRedacted ReasoningBlockKind = chatReasoningRedacted
)

func NewReasoningPart(text string, signature []byte) (corechat.Part, error) {
	if text == "" || len(signature) == 0 {
		return corechat.Part{}, errors.New("bedrock: reasoning text and signature are required")
	}
	part := corechat.NewReasoningPart(text, signature)
	if err := setReasoningKind(&part, chatReasoningText); err != nil {
		return corechat.Part{}, err
	}
	return part, nil
}

func NewRedactedReasoningPart(content []byte) (corechat.Part, error) {
	if len(content) == 0 {
		return corechat.Part{}, errors.New("bedrock: redacted reasoning content is required")
	}
	part := corechat.NewReasoningPart("", content)
	if err := setReasoningKind(&part, chatReasoningRedacted); err != nil {
		return corechat.Part{}, err
	}
	return part, nil
}

// ReasoningBlockKindOf reports whether part contains Bedrock-issued reasoning
// replay state.
func ReasoningBlockKindOf(part corechat.Part) (ReasoningBlockKind, bool, error) {
	value, found, err := part.Metadata.Decode[string](chatReasoningKindKey)
	if err != nil {
		return "", found, fmt.Errorf("bedrock: reasoning metadata: %w", err)
	}
	if !found {
		return "", false, nil
	}
	kind := ReasoningBlockKind(value)
	switch kind {
	case ReasoningBlockText, ReasoningBlockRedacted:
		return kind, true, nil
	default:
		return "", true, fmt.Errorf("bedrock: unknown reasoning block kind %q", value)
	}
}

func setReasoningKind(part *corechat.Part, kind string) error {
	if err := part.Metadata.Set(chatReasoningKindKey, kind); err != nil {
		return fmt.Errorf("bedrock: preserve reasoning kind: %w", err)
	}
	return nil
}

func setReasoningDeltaKind(part *corechat.PartDelta, kind string) error {
	if err := part.Metadata.Set(chatReasoningKindKey, kind); err != nil {
		return fmt.Errorf("bedrock: preserve reasoning delta kind: %w", err)
	}
	return nil
}
