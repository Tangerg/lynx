package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/internal/extension"
	"github.com/Tangerg/lynx/core/metadata"
)

var (
	// ErrInvalidRequest reports a malformed chat request.
	ErrInvalidRequest = errors.New("chat: invalid request")
	// ErrInvalidExtension reports an extension key outside the namespace/name
	// convention.
	ErrInvalidExtension = errors.New("chat: invalid extension")
)

// Request is the complete provider-neutral input to a chat model. It contains
// only serializable protocol values; executable tools and invocation state are
// supplied separately by higher-level runtimes.
type Request struct {
	Messages   []Message        `json:"messages"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
	Options    Options          `json:"options,omitzero"`
	Extensions metadata.Map     `json:"extensions,omitzero"`
}

// Clone returns an independent copy of r. It is nil-safe.
func (r *Request) Clone() *Request {
	if r == nil {
		return nil
	}
	clone := &Request{
		Messages:   make([]Message, len(r.Messages)),
		Tools:      make([]ToolDefinition, len(r.Tools)),
		Options:    r.Options.Clone(),
		Extensions: r.Extensions.Clone(),
	}
	for index := range r.Messages {
		clone.Messages[index] = r.Messages[index].Clone()
	}
	for index := range r.Tools {
		clone.Tools[index] = r.Tools[index].Clone()
	}
	return clone
}

// NewRequest validates and copies messages into a Request.
func NewRequest(messages ...Message) (*Request, error) {
	r := &Request{Messages: slices.Clone(messages)}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

// SetExtension JSON-encodes value and stores it under a namespace/name key.
// Failed encodes do not modify the request.
func (r *Request) SetExtension(key string, value any) error {
	if r == nil {
		return fmt.Errorf("%w: nil request", ErrInvalidRequest)
	}
	return setExtension(&r.Extensions, key, value)
}

func setExtension(target *metadata.Map, key string, value any) error {
	if err := extension.Set(target, key, value); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExtension, err)
	}
	return nil
}

// Validate recursively verifies messages, tool definitions, options, and
// extensions. Tool names must be unique within one request.
func (r *Request) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil request", ErrInvalidRequest)
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("%w: at least one message is required", ErrInvalidRequest)
	}
	for i := range r.Messages {
		if err := r.Messages[i].Validate(); err != nil {
			return fmt.Errorf("%w: messages[%d]: %w", ErrInvalidRequest, i, err)
		}
	}

	toolNames := make(map[string]struct{}, len(r.Tools))
	for i := range r.Tools {
		if err := r.Tools[i].Validate(); err != nil {
			return fmt.Errorf("%w: tools[%d]: %w", ErrInvalidRequest, i, err)
		}
		if _, duplicate := toolNames[r.Tools[i].Name]; duplicate {
			return fmt.Errorf("%w: duplicate tool name %q", ErrInvalidRequest, r.Tools[i].Name)
		}
		toolNames[r.Tools[i].Name] = struct{}{}
	}
	if err := r.Options.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	if err := validateExtensions(r.Extensions); err != nil {
		return fmt.Errorf("%w: extensions: %w", ErrInvalidRequest, err)
	}
	return nil
}

func validateExtensions(extensions metadata.Map) error {
	if err := extension.Validate(extensions); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExtension, err)
	}
	return nil
}

// MarshalJSON validates Request before writing its wire representation.
func (r Request) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireRequest Request
	return json.Marshal(wireRequest(r))
}

// UnmarshalJSON decodes and validates Request before replacing the receiver.
func (r *Request) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidRequest)
	}
	type wireRequest Request
	var decoded wireRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidRequest, err)
	}
	candidate := Request(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}
