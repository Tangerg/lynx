package chat

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/internal/ptr"
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
	if !chunk.Usage.isZero() {
		a.response.Usage = chunk.Usage.clone()
	}
	if err := a.response.Extensions.Merge(chunk.Extensions); err != nil {
		return fmt.Errorf("chat: accumulate response extensions: %w", err)
	}

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
	response := a.response.cloneHeader()
	response.Choices = make([]Choice, len(a.choices))
	for i := range a.choices {
		response.Choices[i] = a.choices[i].choice.clone()
	}
	return &response
}

func (a *ResponseAccumulator) clone() ResponseAccumulator {
	if a == nil {
		return ResponseAccumulator{}
	}
	clone := ResponseAccumulator{
		response: a.response.cloneHeader(),
		choices:  make([]accumulatedChoice, len(a.choices)),
		byIndex:  make(map[int]int, len(a.byIndex)),
		seen:     a.seen,
	}
	for index, position := range a.byIndex {
		clone.byIndex[index] = position
	}
	for i := range a.choices {
		clone.choices[i] = accumulatedChoice{
			choice:    a.choices[i].choice.clone(),
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
	if err := a.choice.Extensions.Merge(delta.Extensions); err != nil {
		return fmt.Errorf("choice extensions: %w", err)
	}
	if delta.Message == nil {
		return nil
	}
	if a.choice.Message == nil {
		a.choice.Message = &Message{Role: RoleAssistant}
	}
	if err := a.choice.Message.Metadata.Merge(delta.Message.Metadata); err != nil {
		return fmt.Errorf("message metadata: %w", err)
	}
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
		cloned := delta.Clone()
		a.toolParts[delta.ToolCall.ID] = len(*parts)
		*parts = append(*parts, cloned)
		return nil
	}
	*parts = append(*parts, delta.Clone())
	return nil
}

// cloneHeader deep-copies every response field except Choices. The accumulator
// carries choices separately in its own indexed slice.
func (r Response) cloneHeader() Response {
	return Response{
		ID:         r.ID,
		Model:      r.Model,
		Usage:      r.Usage.clone(),
		Extensions: r.Extensions.Clone(),
	}
}

func (c Choice) clone() Choice {
	clone := Choice{
		Index:        c.Index,
		FinishReason: c.FinishReason,
		Extensions:   c.Extensions.Clone(),
	}
	if c.Message != nil {
		clone.Message = new(c.Message.Clone())
	}
	return clone
}

func (u Usage) clone() Usage {
	clone := u
	clone.ReasoningTokens = ptr.Clone(u.ReasoningTokens)
	clone.CacheReadInputTokens = ptr.Clone(u.CacheReadInputTokens)
	clone.CacheWriteInputTokens = ptr.Clone(u.CacheWriteInputTokens)
	return clone
}

func (u Usage) isZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.ReasoningTokens == nil &&
		u.CacheReadInputTokens == nil && u.CacheWriteInputTokens == nil
}
