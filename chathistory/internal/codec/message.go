// Package codec owns the durable history wire boundary shared by every
// chathistory backend.
package codec

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

var (
	// ErrUnknownWire reports a history record without a recognized role
	// discriminator.
	ErrUnknownWire = errors.New("chathistory codec: unknown message wire")
	// ErrAmbiguousWire reports a record containing both the current role and
	// legacy type discriminators.
	ErrAmbiguousWire = errors.New("chathistory codec: ambiguous message wire")
)

// EncodeMessage validates and writes the current core/chat tagged wire. New
// records always use the role discriminator; legacy type-tagged records are a
// read-only compatibility concern in DecodeMessage.
func EncodeMessage(message chat.Message) ([]byte, error) {
	if err := message.Validate(); err != nil {
		return nil, fmt.Errorf("chathistory codec: encode: %w", err)
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("chathistory codec: encode: %w", err)
	}
	return raw, nil
}

// DecodeMessage accepts both the current role-tagged core/chat wire and the
// former type-tagged core/model/chat canonical wire. Successful results are
// always current protocol values.
func DecodeMessage(raw []byte) (chat.Message, error) {
	var head struct {
		Role chat.Role `json:"role"`
		Type string    `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return chat.Message{}, fmt.Errorf("chathistory codec: decode discriminator: %w", err)
	}
	if head.Role != "" && head.Type != "" {
		return chat.Message{}, fmt.Errorf("%w: role %q and type %q", ErrAmbiguousWire, head.Role, head.Type)
	}
	if head.Role != "" {
		var message chat.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			return chat.Message{}, fmt.Errorf("chathistory codec: decode current wire: %w", err)
		}
		return message, nil
	}
	if head.Type != "" {
		message, err := decodeLegacyMessage(raw)
		if err != nil {
			return chat.Message{}, fmt.Errorf("chathistory codec: decode legacy wire: %w", err)
		}
		return message, nil
	}
	return chat.Message{}, fmt.Errorf("%w: missing role or type discriminator", ErrUnknownWire)
}

type legacyMessage struct {
	Type        string             `json:"type"`
	Text        string             `json:"text"`
	Parts       []legacyPart       `json:"parts"`
	Metadata    metadata.Map       `json:"metadata"`
	Media       []*media.Media     `json:"media"`
	ToolReturns []legacyToolReturn `json:"tool_returns"`
}

type legacyPart struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Signature []byte `json:"signature"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type legacyToolReturn struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Result string `json:"result"`
}

func decodeLegacyMessage(raw []byte) (chat.Message, error) {
	var legacy legacyMessage
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return chat.Message{}, err
	}

	message := chat.Message{Metadata: legacy.Metadata}
	switch legacy.Type {
	case "system":
		message.Role = chat.RoleSystem
		message.Parts = []chat.Part{chat.NewTextPart(legacy.Text)}
	case "user":
		message.Role = chat.RoleUser
		if legacy.Text != "" {
			message.Parts = append(message.Parts, chat.NewTextPart(legacy.Text))
		}
		for _, value := range legacy.Media {
			message.Parts = append(message.Parts, chat.NewMediaPart(value))
		}
	case "assistant":
		message.Role = chat.RoleAssistant
		for i, part := range legacy.Parts {
			converted, err := convertLegacyPart(part)
			if err != nil {
				return chat.Message{}, fmt.Errorf("parts[%d]: %w", i, err)
			}
			message.Parts = append(message.Parts, converted)
		}
	case "tool":
		message.Role = chat.RoleTool
		for _, result := range legacy.ToolReturns {
			message.Parts = append(message.Parts, chat.NewToolResultPart(chat.ToolResult{
				ID: result.ID, Name: result.Name, Result: result.Result,
			}))
		}
	default:
		return chat.Message{}, fmt.Errorf("%w: legacy type %q", ErrUnknownWire, legacy.Type)
	}

	if err := message.Validate(); err != nil {
		return chat.Message{}, err
	}
	return message, nil
}

func convertLegacyPart(part legacyPart) (chat.Part, error) {
	switch part.Kind {
	case "text":
		return chat.NewTextPart(part.Text), nil
	case "reasoning":
		return chat.NewReasoningPart(part.Text, part.Signature), nil
	case "tool_call":
		return chat.NewToolCallPart(chat.ToolCall{
			ID: part.ID, Name: part.Name, Arguments: part.Arguments,
		}), nil
	default:
		return chat.Part{}, fmt.Errorf("%w: legacy part kind %q", ErrUnknownWire, part.Kind)
	}
}
