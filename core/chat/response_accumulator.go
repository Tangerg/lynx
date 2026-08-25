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
	result   *accumulatedResult
	seen     bool
}

type accumulatedResult struct {
	result    Result
	toolParts map[string]int
}

// Add validates and atomically merges one stream chunk. An error leaves the
// accumulator unchanged.
func (a *ResponseAccumulator) Add(chunk *Response) error {
	if a == nil {
		return errors.New("chat: nil response accumulator")
	}
	if chunk == nil {
		return fmt.Errorf("%w: nil stream chunk", ErrInvalidResponse)
	}
	if err := chunk.Validate(); err != nil {
		return fmt.Errorf("chat: accumulate: %w", err)
	}

	next := a.clone()
	if err := next.merge(chunk); err != nil {
		return err
	}
	if err := next.snapshot().Validate(); err != nil {
		return fmt.Errorf("chat: accumulated response: %w", err)
	}
	*a = next
	return nil
}

// Response returns an independent snapshot, or nil before the first
// successful Add. Mutating the returned value cannot affect the accumulator.
func (a *ResponseAccumulator) Response() *Response {
	if a == nil || !a.seen {
		return nil
	}
	return a.snapshot()
}

func (a *ResponseAccumulator) merge(chunk *Response) error {
	a.seen = true
	if chunk.Metadata != nil {
		if a.metadata == nil {
			a.metadata = &ResponseMetadata{}
		}
		if err := a.metadata.merge(*chunk.Metadata); err != nil {
			return fmt.Errorf("chat: accumulate response metadata: %w", err)
		}
	}
	if chunk.Result == nil {
		return nil
	}
	if a.result == nil {
		a.result = &accumulatedResult{toolParts: make(map[string]int)}
	}
	if err := a.result.merge(*chunk.Result); err != nil {
		return fmt.Errorf("chat: accumulate result: %w", err)
	}
	return nil
}

func (a *ResponseAccumulator) snapshot() *Response {
	response := &Response{}
	if a.metadata != nil {
		response.Metadata = a.metadata.clone()
	}
	if a.result != nil {
		response.Result = a.result.result.clone()
	}
	return response
}

func (a *ResponseAccumulator) clone() ResponseAccumulator {
	if a == nil {
		return ResponseAccumulator{}
	}
	clone := ResponseAccumulator{seen: a.seen}
	if a.metadata != nil {
		clone.metadata = a.metadata.clone()
	}
	if a.result != nil {
		clone.result = &accumulatedResult{
			result:    *a.result.result.clone(),
			toolParts: maps.Clone(a.result.toolParts),
		}
	}
	return clone
}

func (a *accumulatedResult) merge(delta Result) error {
	if delta.FinishReason != "" {
		a.result.FinishReason = delta.FinishReason
	}
	if delta.Metadata != nil {
		if a.result.Metadata == nil {
			a.result.Metadata = &ResultMetadata{}
		}
		if err := a.result.Metadata.Extra.Merge(delta.Metadata.Extra); err != nil {
			return fmt.Errorf("result metadata: %w", err)
		}
	}
	if delta.Message == nil {
		return nil
	}
	if a.result.Message == nil {
		a.result.Message = &Message{Role: RoleAssistant}
	}
	if err := a.result.Message.Metadata.Merge(delta.Message.Metadata); err != nil {
		return fmt.Errorf("message metadata: %w", err)
	}
	for i := range delta.Message.Parts {
		if err := a.mergePart(delta.Message.Parts[i]); err != nil {
			return fmt.Errorf("part %d: %w", i, err)
		}
	}
	return nil
}

func (a *accumulatedResult) mergePart(delta Part) error {
	parts := &a.result.Message.Parts
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
	case PartToolCall:
		if position, exists := a.toolParts[delta.ToolCall.ID]; exists {
			part := &(*parts)[position]
			call := part.ToolCall
			if call.Name != delta.ToolCall.Name {
				return fmt.Errorf("tool call %q changed name from %q to %q", call.ID, call.Name, delta.ToolCall.Name)
			}
			call.Arguments += delta.ToolCall.Arguments
			if err := part.Metadata.Merge(delta.Metadata); err != nil {
				return fmt.Errorf("tool call %q metadata: %w", call.ID, err)
			}
			return nil
		}
		cloned := delta.Clone()
		a.toolParts[delta.ToolCall.ID] = len(*parts)
		*parts = append(*parts, cloned)
		return nil
	}
	*parts = append(*parts, delta.Clone())
	return nil
}
