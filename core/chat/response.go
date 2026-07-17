package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

var (
	// ErrInvalidResponse reports malformed provider response data.
	ErrInvalidResponse = errors.New("chat: invalid response")
	// ErrInvalidChoice reports a malformed generation choice.
	ErrInvalidChoice = errors.New("chat: invalid choice")
)

// FinishReason explains why generation stopped. The empty value means that a
// streaming choice has not finished yet.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonOther         FinishReason = "other"
)

func (r FinishReason) String() string { return string(r) }

// Valid reports whether r is empty (not finished) or a known normalized
// finish reason. Provider-native reasons map to Other and a choice extension.
func (r FinishReason) Valid() bool {
	switch r {
	case "", FinishReasonStop, FinishReasonLength, FinishReasonToolCalls, FinishReasonContentFilter, FinishReasonOther:
		return true
	default:
		return false
	}
}

// Choice is one provider generation. Message may be nil on a streaming chunk
// that only carries a finish reason or choice extensions.
type Choice struct {
	Index        int          `json:"index"`
	Message      *Message     `json:"message,omitempty"`
	FinishReason FinishReason `json:"finish_reason,omitempty"`
	Extensions   metadata.Map `json:"extensions,omitzero"`
}

// SetExtension JSON-encodes a choice-scoped provider value.
func (c *Choice) SetExtension(key string, value any) error {
	if c == nil {
		return fmt.Errorf("%w: nil choice", ErrInvalidChoice)
	}
	return setExtension(&c.Extensions, key, value)
}

// Text returns the choice's assistant text, or an empty string when absent.
func (c *Choice) Text() string {
	if c == nil || c.Message == nil {
		return ""
	}
	return c.Message.Text()
}

// Validate verifies choice identity, normalized finish reason, assistant
// message content, and JSON-safe extensions.
func (c *Choice) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: nil choice", ErrInvalidChoice)
	}
	if c.Index < 0 {
		return fmt.Errorf("%w: index must not be negative", ErrInvalidChoice)
	}
	if c.Message == nil && c.FinishReason == "" && len(c.Extensions) == 0 {
		return fmt.Errorf("%w: choice has no message, finish reason, or extensions", ErrInvalidChoice)
	}
	if c.Message != nil {
		if err := c.Message.Validate(); err != nil {
			return fmt.Errorf("%w: message: %w", ErrInvalidChoice, err)
		}
		if c.Message.Role != RoleAssistant {
			return fmt.Errorf("%w: message role must be %q, got %q", ErrInvalidChoice, RoleAssistant, c.Message.Role)
		}
	}
	if !c.FinishReason.Valid() {
		return fmt.Errorf("%w: unknown finish reason %q", ErrInvalidChoice, c.FinishReason)
	}
	if err := validateExtensions(c.Extensions); err != nil {
		return fmt.Errorf("%w: extensions: %w", ErrInvalidChoice, err)
	}
	return nil
}

// MarshalJSON validates Choice before writing its wire representation.
func (c Choice) MarshalJSON() ([]byte, error) {
	if err := (&c).Validate(); err != nil {
		return nil, err
	}
	type wireChoice Choice
	return json.Marshal(wireChoice(c))
}

// UnmarshalJSON decodes and validates Choice before replacing the receiver.
func (c *Choice) UnmarshalJSON(data []byte) error {
	if c == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidChoice)
	}
	type wireChoice Choice
	var decoded wireChoice
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidChoice, err)
	}
	candidate := Choice(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*c = candidate
	return nil
}

// Response is provider output. It never contains tool execution results,
// pause/resume signals, or other orchestration events. Its zero value is valid
// so a stream can represent an empty or usage-only chunk.
type Response struct {
	ID         string       `json:"id,omitempty"`
	Model      string       `json:"model,omitempty"`
	Choices    []Choice     `json:"choices,omitempty"`
	Usage      Usage        `json:"usage,omitzero"`
	Extensions metadata.Map `json:"extensions,omitzero"`
}

// NewResponse validates a Response containing choices.
func NewResponse(choices ...Choice) (*Response, error) {
	response := &Response{Choices: append([]Choice(nil), choices...)}
	if err := response.Validate(); err != nil {
		return nil, err
	}
	return response, nil
}

// SetExtension JSON-encodes a response-scoped provider value.
func (r *Response) SetExtension(key string, value any) error {
	if r == nil {
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	return setExtension(&r.Extensions, key, value)
}

// First returns the first provider choice or nil when no choice is present.
func (r *Response) First() *Choice {
	if r == nil || len(r.Choices) == 0 {
		return nil
	}
	return &r.Choices[0]
}

// Text returns the first choice's assistant text. It is nil/empty-safe.
func (r *Response) Text() string {
	return r.First().Text()
}

// Validate recursively verifies response data. Empty Choices is valid for
// stream chunks that only carry usage or provider metadata.
func (r *Response) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if r.ID != "" && strings.TrimSpace(r.ID) != r.ID {
		return fmt.Errorf("%w: ID must not have surrounding whitespace", ErrInvalidResponse)
	}
	if r.Model != "" && strings.TrimSpace(r.Model) != r.Model {
		return fmt.Errorf("%w: model must not have surrounding whitespace", ErrInvalidResponse)
	}
	indices := make(map[int]struct{}, len(r.Choices))
	for i := range r.Choices {
		if err := r.Choices[i].Validate(); err != nil {
			return fmt.Errorf("%w: choices[%d]: %w", ErrInvalidResponse, i, err)
		}
		if _, duplicate := indices[r.Choices[i].Index]; duplicate {
			return fmt.Errorf("%w: duplicate choice index %d", ErrInvalidResponse, r.Choices[i].Index)
		}
		indices[r.Choices[i].Index] = struct{}{}
	}
	if err := r.Usage.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	if err := validateExtensions(r.Extensions); err != nil {
		return fmt.Errorf("%w: extensions: %w", ErrInvalidResponse, err)
	}
	return nil
}

// MarshalJSON validates Response before writing its wire representation.
func (r Response) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireResponse Response
	return json.Marshal(wireResponse(r))
}

// UnmarshalJSON decodes and validates Response before replacing the receiver.
func (r *Response) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidResponse)
	}
	type wireResponse Response
	var decoded wireResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidResponse, err)
	}
	candidate := Response(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}
