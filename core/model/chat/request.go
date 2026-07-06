package chat

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Request is a chat completion request: the conversation history, the
// tools the model may invoke, the model options, and free-form
// per-request parameters (user id, session id, etc. — whatever
// middleware wants to thread through).
type Request struct {
	// Messages is the conversation history sent to the model.
	Messages []Message `json:"messages,omitzero"`

	// Tools the model may invoke during generation. Excluded from JSON
	// to keep the wire format provider-agnostic — serialization is the
	// provider's job. Sits at the Request level (not inside Options)
	// because tools are capability, not sampling configuration.
	Tools []Tool `json:"-"`

	// Options carries model-specific parameters.
	Options *Options `json:"options,omitempty"`

	// Params holds caller-supplied side-channel data (user id, trace id,
	// feature flags) that middlewares can read.
	Params map[string]any `json:"params,omitzero"`
}

// NewRequest builds a Request from the given messages. nil entries are
// filtered out; an empty (or nil-only) input returns an error.
//
// Example:
//
//	req, err := chat.NewRequest([]chat.Message{
//	    chat.NewSystemMessage("You are concise."),
//	    chat.NewUserMessage("Hello"),
//	})
func NewRequest(messages []Message) (*Request, error) {
	valid := MessageList(messages).withoutNil()
	if len(valid) == 0 {
		return nil, errors.New("chat.NewRequest: messages must contain at least one non-nil entry")
	}

	return &Request{
		Messages: valid,
		Params:   make(map[string]any),
	}, nil
}

func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

func (r *Request) Get(key string) (any, bool) {
	if r == nil || r.Params == nil {
		return nil, false
	}
	value, exists := r.Params[key]
	return value, exists
}

func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}

// AppendToLastUserMessage appends text to the last user message, separated
// from existing content by a double newline. No-op when no user message
// exists.
func (r *Request) AppendToLastUserMessage(text string) {
	MessageList(r.Messages).appendTextToLastOfType(MessageTypeUser, text)
}

// ReplaceTextOfLastUserMessage replaces the entire text of the last
// user message. No-op when no user message exists.
func (r *Request) ReplaceTextOfLastUserMessage(text string) {
	MessageList(r.Messages).replaceTextOfLastOfType(MessageTypeUser, text)
}

// UserMessage returns the most-recent user message in the conversation,
// or an empty *UserMessage if the conversation has none.
func (r *Request) UserMessage() *UserMessage {
	if r == nil {
		return NewUserMessage("")
	}
	idx, last := MessageList(r.Messages).lastOfType(MessageTypeUser)
	if idx == -1 {
		return NewUserMessage("")
	}
	return last.(*UserMessage)
}

// SystemMessage returns the most-recent system message in the
// conversation, or an empty *SystemMessage if the conversation has none.
func (r *Request) SystemMessage() *SystemMessage {
	if r == nil {
		return NewSystemMessage("")
	}
	idx, last := MessageList(r.Messages).lastOfType(MessageTypeSystem)
	if idx == -1 {
		return NewSystemMessage("")
	}
	return last.(*SystemMessage)
}

// UnmarshalJSON decodes a Request, dispatching each entry in the
// "messages" array to the concrete [Message] type indicated by its
// "type" discriminator. The standard library cannot decode into a
// []Message interface slice by itself, so the polymorphism lives
// here rather than scattered across every caller. Tools intentionally
// don't round-trip — they hold runtime closures and are excluded from
// JSON by design.
func (r *Request) UnmarshalJSON(data []byte) error {
	var raw struct {
		Messages []json.RawMessage `json:"messages,omitzero"`
		Options  *Options          `json:"options,omitempty"`
		Params   map[string]any    `json:"params,omitzero"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Options = raw.Options
	r.Params = raw.Params
	if len(raw.Messages) == 0 {
		r.Messages = nil
		return nil
	}

	r.Messages = make([]Message, 0, len(raw.Messages))
	for i, m := range raw.Messages {
		msg, err := UnmarshalMessage(m)
		if err != nil {
			return fmt.Errorf("chat.Request.UnmarshalJSON: messages[%d]: %w", i, err)
		}
		r.Messages = append(r.Messages, msg)
	}
	return nil
}
