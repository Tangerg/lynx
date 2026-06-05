package chat

import (
	"encoding/json"
	"fmt"
)

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
