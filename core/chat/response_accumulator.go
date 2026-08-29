package chat

import (
	"errors"
	"fmt"
	"maps"
)

// ResponseAccumulator merges response deltas into one provider-neutral
// response. Its zero value is ready to use and it never mutates supplied
// chunks.
//
// Text and reasoning merge only while adjacent. Tool-call arguments merge by
// stable call ID even when parallel calls are interleaved. Identity and finish
// fields use the latest non-empty value, metadata merges last-write-wins, and
// Usage is a cumulative snapshot whose latest non-zero value replaces the
// previous snapshot.
type ResponseAccumulator struct {
	metadata *ResponseMetadata
	output   *accumulatedOutput
	seen     bool
}

type accumulatedOutput struct {
	output    Output
	toolParts map[string]int
}

// Add validates and atomically merges one stream chunk. An error leaves the
// accumulator unchanged.
func (r *ResponseAccumulator) Add(chunk *Response) error {
	if r == nil {
		return errors.New("chat: nil response accumulator")
	}
	if chunk == nil {
		return fmt.Errorf("%w: nil stream chunk", ErrInvalidResponse)
	}
	if err := chunk.Validate(); err != nil {
		return fmt.Errorf("chat: accumulate: %w", err)
	}

	next := r.clone()
	if err := next.merge(chunk); err != nil {
		return err
	}
	if err := next.snapshot().Validate(); err != nil {
		return fmt.Errorf("chat: accumulated response: %w", err)
	}
	*r = next
	return nil
}

// Response returns an independent snapshot, or nil before the first
// successful Add. Mutating the returned value cannot affect the accumulator.
func (r *ResponseAccumulator) Response() *Response {
	if r == nil || !r.seen {
		return nil
	}
	return r.snapshot()
}

func (r *ResponseAccumulator) merge(chunk *Response) error {
	r.seen = true
	if chunk.Metadata != nil {
		if r.metadata == nil {
			r.metadata = &ResponseMetadata{}
		}
		if err := r.metadata.merge(*chunk.Metadata); err != nil {
			return fmt.Errorf("chat: accumulate response metadata: %w", err)
		}
	}
	if chunk.Output == nil {
		return nil
	}
	if r.output == nil {
		r.output = &accumulatedOutput{toolParts: make(map[string]int)}
	}
	if err := r.output.merge(*chunk.Output); err != nil {
		return fmt.Errorf("chat: accumulate output: %w", err)
	}
	return nil
}

func (r *ResponseAccumulator) snapshot() *Response {
	response := &Response{}
	if r.metadata != nil {
		response.Metadata = r.metadata.clone()
	}
	if r.output != nil {
		response.Output = r.output.output.clone()
	}
	return response
}

func (r *ResponseAccumulator) clone() ResponseAccumulator {
	if r == nil {
		return ResponseAccumulator{}
	}
	clone := ResponseAccumulator{seen: r.seen}
	if r.metadata != nil {
		clone.metadata = r.metadata.clone()
	}
	if r.output != nil {
		clone.output = &accumulatedOutput{
			output:    *r.output.output.clone(),
			toolParts: maps.Clone(r.output.toolParts),
		}
	}
	return clone
}

func (a *accumulatedOutput) merge(delta Output) error {
	if delta.FinishReason != "" {
		a.output.FinishReason = delta.FinishReason
	}
	if delta.Metadata != nil {
		if a.output.Metadata == nil {
			a.output.Metadata = &OutputMetadata{}
		}
		if err := a.output.Metadata.Extra.Merge(delta.Metadata.Extra); err != nil {
			return fmt.Errorf("output metadata: %w", err)
		}
	}
	if delta.Message == nil {
		return nil
	}
	if a.output.Message == nil {
		a.output.Message = &Message{Role: RoleAssistant}
	}
	if err := a.output.Message.Metadata.Merge(delta.Message.Metadata); err != nil {
		return fmt.Errorf("message metadata: %w", err)
	}
	for i := range delta.Message.Parts {
		if err := a.mergePart(delta.Message.Parts[i]); err != nil {
			return fmt.Errorf("part %d: %w", i, err)
		}
	}
	return nil
}

func (a *accumulatedOutput) mergePart(delta Part) error {
	parts := &a.output.Message.Parts
	switch delta.Kind {
	case PartText:
		if len(*parts) > 0 && (*parts)[len(*parts)-1].Kind == PartText && (*parts)[len(*parts)-1].Metadata.Equal(delta.Metadata) {
			(*parts)[len(*parts)-1].Text += delta.Text
			return nil
		}
	case PartReasoning:
		if len(*parts) > 0 && (*parts)[len(*parts)-1].Kind == PartReasoning && (*parts)[len(*parts)-1].Metadata.Equal(delta.Metadata) {
			last := &(*parts)[len(*parts)-1]
			last.Text += delta.Text
			last.Signature = append(last.Signature, delta.Signature...)
			return nil
		}
	case PartToolCallDelta:
		callDelta := delta.ToolCallDelta
		if position, exists := a.toolParts[callDelta.ID]; exists {
			part := &(*parts)[position]
			call := part.ToolCall
			if call.Name != callDelta.Name {
				return fmt.Errorf("tool call %q changed name from %q to %q", call.ID, call.Name, callDelta.Name)
			}
			call.Arguments += callDelta.Arguments
			if err := part.Metadata.Merge(delta.Metadata); err != nil {
				return fmt.Errorf("tool call %q metadata: %w", call.ID, err)
			}
			return nil
		}
		assembled := NewToolCallPart(ToolCall{ID: callDelta.ID, Name: callDelta.Name, Arguments: callDelta.Arguments})
		assembled.Metadata = delta.Metadata.Clone()
		a.toolParts[callDelta.ID] = len(*parts)
		*parts = append(*parts, assembled)
		return nil
	case PartToolCall:
		if _, exists := a.toolParts[delta.ToolCall.ID]; exists {
			return fmt.Errorf("tool call %q was emitted more than once or mixed with deltas", delta.ToolCall.ID)
		}
		a.toolParts[delta.ToolCall.ID] = len(*parts)
		*parts = append(*parts, delta.Clone())
		return nil
	}
	*parts = append(*parts, delta.Clone())
	return nil
}
