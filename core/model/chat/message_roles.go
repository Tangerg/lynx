package chat

import (
	"errors"

	"github.com/Tangerg/lynx/core/media"
)

// SystemMessage shapes the model's behavior for the whole conversation —
// persona, response format, guardrails. It typically sits at the head
// of the message list.
type SystemMessage struct {
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitzero"`
}

func (s *SystemMessage) message() {}

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

func (u *UserMessage) Type() MessageType { return MessageTypeUser }

// Meta returns the metadata map, allocating it on first access.
func (u *UserMessage) Meta() map[string]any {
	if u.Metadata == nil {
		u.Metadata = make(map[string]any)
	}
	return u.Metadata
}

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
