package chat

import (
	"errors"
	"fmt"
	"maps"
)

// ResponseAccumulator is the only promotion path from transport deltas to a
// complete Response. Its zero value is ready to use and Add is atomic.
type ResponseAccumulator struct {
	metadata       *ResponseMetadata
	message        *Message
	outputMetadata *OutputMetadata
	finishReason   FinishReason
	toolParts      map[string]int
	seen           bool
}

// Add validates and applies one delta atomically; a failed merge leaves the
// accumulated stream unchanged.
func (r *ResponseAccumulator) Add(delta *ResponseDelta) error {
	if r == nil {
		return errors.New("chat: nil response accumulator")
	}
	if err := delta.Validate(); err != nil {
		return fmt.Errorf("chat: accumulate: %w", err)
	}
	next := r.clone()
	if err := next.merge(delta); err != nil {
		return fmt.Errorf("chat: accumulate: %w: %w", ErrInvalidResponse, err)
	}
	*r = next
	return nil
}

// Text returns the currently accumulated visible text without manufacturing a
// partial Response.
func (r *ResponseAccumulator) Text() string {
	if r == nil || r.message == nil {
		return ""
	}
	return r.message.Text()
}

// Response promotes the accumulated stream only after a terminal finish reason
// has been observed.
func (r *ResponseAccumulator) Response() (*Response, error) {
	if r == nil || !r.seen {
		return nil, fmt.Errorf("%w: stream produced no deltas", ErrInvalidResponse)
	}
	if r.finishReason == "" {
		return nil, fmt.Errorf("%w: stream ended without a finish reason", ErrInvalidResponse)
	}
	output := &Output{FinishReason: r.finishReason}
	if r.message != nil {
		message := r.message.Clone()
		output.Message = &message
	}
	if r.outputMetadata != nil {
		output.Metadata = r.outputMetadata.clone()
	}
	response := &Response{Output: output}
	if r.metadata != nil {
		response.Metadata = r.metadata.clone()
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("chat: accumulated response: %w", err)
	}
	return response, nil
}

func (r *ResponseAccumulator) merge(delta *ResponseDelta) error {
	if r.finishReason != "" {
		return errors.New("stream emitted a delta after its finish reason")
	}
	r.seen = true
	if delta.Metadata != nil {
		if r.metadata == nil {
			r.metadata = &ResponseMetadata{}
		}
		if err := r.metadata.merge(*delta.Metadata); err != nil {
			return fmt.Errorf("chat: accumulate response metadata: %w", err)
		}
	}
	if delta.OutputMetadata != nil {
		if r.outputMetadata == nil {
			r.outputMetadata = &OutputMetadata{}
		}
		if err := r.outputMetadata.Extra.Merge(delta.OutputMetadata.Extra); err != nil {
			return fmt.Errorf("chat: accumulate output metadata: %w", err)
		}
	}
	if delta.FinishReason != "" {
		r.finishReason = delta.FinishReason
	}
	if len(delta.Parts) == 0 && len(delta.MessageMetadata) == 0 {
		return nil
	}
	if r.message == nil {
		r.message = &Message{Role: RoleAssistant}
	}
	if err := r.message.Metadata.Merge(delta.MessageMetadata); err != nil {
		return fmt.Errorf("chat: accumulate message metadata: %w", err)
	}
	if r.toolParts == nil {
		r.toolParts = make(map[string]int)
	}
	for index := range delta.Parts {
		if err := r.mergePart(delta.Parts[index]); err != nil {
			return fmt.Errorf("chat: accumulate part %d: %w", index, err)
		}
	}
	return nil
}

func (r *ResponseAccumulator) mergePart(delta PartDelta) error {
	parts := &r.message.Parts
	switch delta.Kind {
	case PartDeltaText:
		return r.mergeTextLike(parts, PartText, delta)
	case PartDeltaMedia:
		part := NewMediaPart(delta.Media.Clone())
		part.Metadata = delta.Metadata.Clone()
		*parts = append(*parts, part)
		return nil
	case PartDeltaReasoning:
		if len(*parts) > 0 && (*parts)[len(*parts)-1].Kind == PartReasoning && (*parts)[len(*parts)-1].Metadata.Equal(delta.Metadata) {
			last := &(*parts)[len(*parts)-1]
			last.Text += delta.Text
			last.ReasoningState = append(last.ReasoningState, delta.ReasoningState...)
			return nil
		}
		part := NewReasoningPart(delta.Text, delta.ReasoningState)
		part.Metadata = delta.Metadata.Clone()
		*parts = append(*parts, part)
		return nil
	case PartDeltaToolCall:
		callDelta := delta.ToolCall
		if position, exists := r.toolParts[callDelta.ID]; exists {
			part := &(*parts)[position]
			if part.ToolCall.Name != callDelta.Name {
				return fmt.Errorf("tool call %q changed name from %q to %q", part.ToolCall.ID, part.ToolCall.Name, callDelta.Name)
			}
			part.ToolCall.Arguments += callDelta.Arguments
			if err := part.Metadata.Merge(delta.Metadata); err != nil {
				return fmt.Errorf("tool call %q metadata: %w", part.ToolCall.ID, err)
			}
			return nil
		}
		part := NewToolCallPart(ToolCall{ID: callDelta.ID, Name: callDelta.Name, Arguments: callDelta.Arguments})
		part.Metadata = delta.Metadata.Clone()
		r.toolParts[callDelta.ID] = len(*parts)
		*parts = append(*parts, part)
		return nil
	case PartDeltaCitation:
		if len(*parts) == 0 || (*parts)[len(*parts)-1].Kind != PartText {
			return errors.New("citation delta does not follow text")
		}
		(*parts)[len(*parts)-1].Citations = append((*parts)[len(*parts)-1].Citations, delta.Citation.Clone())
		return nil
	case PartDeltaRefusal:
		return r.mergeTextLike(parts, PartRefusal, delta)
	default:
		return fmt.Errorf("unknown delta kind %q", delta.Kind)
	}
}

func (r *ResponseAccumulator) mergeTextLike(parts *[]Part, kind PartKind, delta PartDelta) error {
	if len(*parts) > 0 && (*parts)[len(*parts)-1].Kind == kind && (*parts)[len(*parts)-1].Metadata.Equal(delta.Metadata) {
		(*parts)[len(*parts)-1].Text += delta.Text
		return nil
	}
	part := Part{Kind: kind, Text: delta.Text, Metadata: delta.Metadata.Clone()}
	*parts = append(*parts, part)
	return nil
}

func (r *ResponseAccumulator) clone() ResponseAccumulator {
	if r == nil {
		return ResponseAccumulator{}
	}
	clone := ResponseAccumulator{
		finishReason: r.finishReason,
		toolParts:    maps.Clone(r.toolParts),
		seen:         r.seen,
	}
	if r.metadata != nil {
		clone.metadata = r.metadata.clone()
	}
	if r.message != nil {
		message := r.message.Clone()
		clone.message = &message
	}
	if r.outputMetadata != nil {
		clone.outputMetadata = r.outputMetadata.clone()
	}
	return clone
}
