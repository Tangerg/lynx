package chat

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"strings"
)

// ToolCall is one complete, untrusted model proposal to invoke a named tool.
// Arguments retains the provider's JSON text so malformed final model output
// remains serializable. A runtime must promote the proposal through the bound
// Tool schema before exposing it to capabilities or execution.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// ToolCallDelta is one streaming fragment of a ToolCall. It cannot be placed in
// a model Request; [ResponseAccumulator] is the boundary that assembles deltas
// into a complete, still-untrusted ToolCall.
type ToolCallDelta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func (t ToolCallDelta) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("%w: delta ID must not be empty", ErrInvalidToolCall)
	}
	if t.Name == "" {
		return fmt.Errorf("%w: delta name must not be empty", ErrInvalidToolCall)
	}
	return nil
}

// ToolOutput is the provider-neutral value produced by a Tool. Content is the
// model-visible ordered text/media representation. Details is optional JSON
// for structured consumers; providers use its encoded JSON as the model-visible
// fallback only when Content is empty.
//
// Content deliberately reuses Part so media has one representation across the
// chat protocol. Only text and media parts are valid here; nested reasoning,
// calls, deltas, and results are rejected.
type ToolOutput struct {
	Content []Part          `json:"content,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`
}

// NewTextToolOutput returns a text output. Empty text is represented by the
// valid zero ToolOutput rather than an invalid empty text Part.
func NewTextToolOutput(text string) ToolOutput {
	if text == "" {
		return ToolOutput{}
	}
	return ToolOutput{Content: []Part{NewTextPart(text)}}
}

// NewJSONToolOutput returns a structured output whose exact JSON encoding is
// preserved. The value must be one complete RFC 7493 JSON document.
func NewJSONToolOutput(value json.RawMessage) (ToolOutput, error) {
	output := ToolOutput{Details: bytes.Clone(value)}
	if err := output.Validate(); err != nil {
		return ToolOutput{}, err
	}
	return output, nil
}

func (o ToolOutput) Clone() ToolOutput {
	clone := ToolOutput{Details: bytes.Clone(o.Details)}
	if o.Content != nil {
		clone.Content = make([]Part, len(o.Content))
		for index := range o.Content {
			clone.Content[index] = o.Content[index].Clone()
		}
	}
	return clone
}

func (o ToolOutput) Validate() error {
	for index := range o.Content {
		part := o.Content[index]
		if part.Kind != PartText && part.Kind != PartMedia {
			return fmt.Errorf("tool output content[%d]: unsupported part kind %q", index, part.Kind)
		}
		if err := part.Validate(); err != nil {
			return fmt.Errorf("tool output content[%d]: %w", index, err)
		}
	}
	if len(o.Details) != 0 && !jsontext.Value(o.Details).IsValid() {
		return fmt.Errorf("tool output details must be one valid RFC 7493 JSON document")
	}
	return nil
}

// Text returns the lossless text projection used by providers whose tool
// result protocol accepts only strings. It reports false when Content contains
// media so adapters cannot silently discard it. Details is encoded only when
// Content is empty.
func (o ToolOutput) Text() (string, bool) {
	if len(o.Content) == 0 {
		return string(o.Details), true
	}
	var projected strings.Builder
	for index := range o.Content {
		if o.Content[index].Kind != PartText {
			return "", false
		}
		projected.WriteString(o.Content[index].Text)
	}
	return projected.String(), true
}

func (t ToolCall) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("%w: ID must not be empty", ErrInvalidToolCall)
	}
	if t.Name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidToolCall)
	}
	return nil
}

// ToolResult is one tool execution result correlated to a ToolCall by ID.
type ToolResult struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Output  ToolOutput `json:"output"`
	IsError bool       `json:"is_error,omitempty"`
}

func (t ToolResult) Clone() ToolResult {
	clone := t
	clone.Output = t.Output.Clone()
	return clone
}

func (t ToolResult) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("%w: ID must not be empty", ErrInvalidToolResult)
	}
	if t.Name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidToolResult)
	}
	if err := t.Output.Validate(); err != nil {
		return fmt.Errorf("%w: output: %w", ErrInvalidToolResult, err)
	}
	return nil
}
