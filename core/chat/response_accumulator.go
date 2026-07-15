package chat

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/metadata"
)

// ResponseAccumulator merges response deltas into one provider-neutral
// response. Its zero value is ready to use. It retains no package-level state
// and never mutates chunks supplied to Add.
//
// Text and reasoning merge only while adjacent within a choice. Tool-call
// arguments merge by stable call ID even when parallel calls are interleaved.
// Identity and finish fields use the latest non-empty value, extensions merge
// last-write-wins, and Usage is a cumulative snapshot whose latest non-zero
// value replaces the previous snapshot.
type ResponseAccumulator struct {
	response Response
	choices  []accumulatedChoice
	byIndex  map[int]int
	seen     bool
}

type accumulatedChoice struct {
	choice    Choice
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
	if chunk.ID != "" {
		a.response.ID = chunk.ID
	}
	if chunk.Model != "" {
		a.response.Model = chunk.Model
	}
	if !zeroUsage(chunk.Usage) {
		a.response.Usage = cloneUsage(chunk.Usage)
	}
	mergeMetadata(&a.response.Extensions, chunk.Extensions)

	if a.byIndex == nil {
		a.byIndex = make(map[int]int)
	}
	for i := range chunk.Choices {
		delta := chunk.Choices[i]
		position, exists := a.byIndex[delta.Index]
		if !exists {
			position = len(a.choices)
			a.byIndex[delta.Index] = position
			a.choices = append(a.choices, accumulatedChoice{
				choice:    Choice{Index: delta.Index},
				toolParts: make(map[string]int),
			})
		}
		if err := a.choices[position].merge(delta); err != nil {
			return fmt.Errorf("chat: accumulate choice %d: %w", delta.Index, err)
		}
	}
	return nil
}

func (a *ResponseAccumulator) snapshot() *Response {
	response := cloneResponseHeader(a.response)
	response.Choices = make([]Choice, len(a.choices))
	for i := range a.choices {
		response.Choices[i] = cloneChoice(a.choices[i].choice)
	}
	return &response
}

func (a *ResponseAccumulator) clone() ResponseAccumulator {
	if a == nil {
		return ResponseAccumulator{}
	}
	clone := ResponseAccumulator{
		response: cloneResponseHeader(a.response),
		choices:  make([]accumulatedChoice, len(a.choices)),
		byIndex:  make(map[int]int, len(a.byIndex)),
		seen:     a.seen,
	}
	for index, position := range a.byIndex {
		clone.byIndex[index] = position
	}
	for i := range a.choices {
		clone.choices[i] = accumulatedChoice{
			choice:    cloneChoice(a.choices[i].choice),
			toolParts: make(map[string]int, len(a.choices[i].toolParts)),
		}
		for id, position := range a.choices[i].toolParts {
			clone.choices[i].toolParts[id] = position
		}
	}
	return clone
}

func (a *accumulatedChoice) merge(delta Choice) error {
	if delta.FinishReason != "" {
		a.choice.FinishReason = delta.FinishReason
	}
	mergeMetadata(&a.choice.Extensions, delta.Extensions)
	if delta.Message == nil {
		return nil
	}
	if a.choice.Message == nil {
		a.choice.Message = &Message{Role: RoleAssistant}
	}
	mergeMetadata(&a.choice.Message.Metadata, delta.Message.Metadata)
	for i := range delta.Message.Parts {
		if err := a.mergePart(delta.Message.Parts[i]); err != nil {
			return fmt.Errorf("part %d: %w", i, err)
		}
	}
	return nil
}

func (a *accumulatedChoice) mergePart(delta Part) error {
	parts := &a.choice.Message.Parts
	switch delta.Kind {
	case PartText:
		if len(*parts) > 0 && (*parts)[len(*parts)-1].Kind == PartText {
			(*parts)[len(*parts)-1].Text += delta.Text
			return nil
		}
	case PartReasoning:
		if len(*parts) > 0 && (*parts)[len(*parts)-1].Kind == PartReasoning {
			last := &(*parts)[len(*parts)-1]
			last.Text += delta.Text
			last.Signature = append(last.Signature, delta.Signature...)
			return nil
		}
	case PartToolCall:
		if position, exists := a.toolParts[delta.ToolCall.ID]; exists {
			call := (*parts)[position].ToolCall
			if call.Name != delta.ToolCall.Name {
				return fmt.Errorf("tool call %q changed name from %q to %q", call.ID, call.Name, delta.ToolCall.Name)
			}
			call.Arguments += delta.ToolCall.Arguments
			return nil
		}
		cloned := clonePart(delta)
		a.toolParts[delta.ToolCall.ID] = len(*parts)
		*parts = append(*parts, cloned)
		return nil
	}
	*parts = append(*parts, clonePart(delta))
	return nil
}

func cloneResponseHeader(response Response) Response {
	return Response{
		ID:         response.ID,
		Model:      response.Model,
		Usage:      cloneUsage(response.Usage),
		Extensions: response.Extensions.Clone(),
	}
}

func cloneChoice(choice Choice) Choice {
	clone := Choice{
		Index:        choice.Index,
		FinishReason: choice.FinishReason,
		Extensions:   choice.Extensions.Clone(),
	}
	if choice.Message != nil {
		clone.Message = &Message{
			Role:     choice.Message.Role,
			Parts:    make([]Part, len(choice.Message.Parts)),
			Metadata: choice.Message.Metadata.Clone(),
		}
		for i := range choice.Message.Parts {
			clone.Message.Parts[i] = clonePart(choice.Message.Parts[i])
		}
	}
	return clone
}

func clonePart(part Part) Part {
	clone := part
	clone.Signature = slices.Clone(part.Signature)
	if part.Media != nil {
		value := *part.Media
		value.Source.Bytes = slices.Clone(part.Media.Source.Bytes)
		value.Metadata = part.Media.Metadata.Clone()
		clone.Media = &value
	}
	if part.ToolCall != nil {
		clone.ToolCall = new(*part.ToolCall)
	}
	return clone
}

func cloneUsage(usage Usage) Usage {
	clone := usage
	clone.ReasoningTokens = clonePointer(usage.ReasoningTokens)
	clone.CacheReadInputTokens = clonePointer(usage.CacheReadInputTokens)
	clone.CacheWriteInputTokens = clonePointer(usage.CacheWriteInputTokens)
	return clone
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

func zeroUsage(usage Usage) bool {
	return usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.ReasoningTokens == nil &&
		usage.CacheReadInputTokens == nil && usage.CacheWriteInputTokens == nil
}

func mergeMetadata(target *metadata.Map, delta metadata.Map) {
	if len(delta) == 0 {
		return
	}
	if *target == nil {
		*target = metadata.New()
	}
	for key, value := range delta {
		(*target)[key] = slices.Clone(value)
	}
}
