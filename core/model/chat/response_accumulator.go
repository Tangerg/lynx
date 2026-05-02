package chat

import (
	"maps"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// ResponseAccumulator stitches a streaming sequence of [*Response] chunks
// back into one full [Response]. It encapsulates the per-field merge rules
// — what concatenates (text, reasoning, tool-call deltas), what overwrites
// (finish reason, response id), and what merges (the Extra maps) — so
// callers can stream a chat reply and consume it as if it had arrived at
// once.
//
// Example:
//
//	acc := chat.NewResponseAccumulator()
//	for chunk, err := range stream.Stream(ctx, req) {
//	    if err != nil { return err }
//	    acc.AddChunk(chunk)
//	}
//	full := &acc.Response
type ResponseAccumulator struct {
	Response
}

// NewResponseAccumulator returns an empty accumulator ready to receive
// chunks via [ResponseAccumulator.AddChunk].
func NewResponseAccumulator() *ResponseAccumulator {
	return &ResponseAccumulator{}
}

// AddChunk merges chunk into the accumulator. Safe to call any number of
// times in the order chunks arrive.
func (r *ResponseAccumulator) AddChunk(chunk *Response) {
	r.accumulateResults(chunk.Results)
	r.accumulateMetadata(chunk.Metadata)
}

// accumulateMetadata merges response-level metadata. Scalars overwrite
// (the latest chunk wins for id/model/usage/rate-limit); the Extra map
// merges last-write-wins.
func (r *ResponseAccumulator) accumulateMetadata(other *ResponseMetadata) {
	if other == nil {
		return
	}
	if r.Metadata == nil {
		r.Metadata = &ResponseMetadata{}
	}

	r.Metadata.ID = other.ID
	r.Metadata.Model = other.Model
	r.Metadata.Usage = other.Usage
	r.Metadata.RateLimit = other.RateLimit
	r.Metadata.Created = other.Created

	r.Metadata.ensureExtra()
	maps.Copy(r.Metadata.Extra, other.Extra)
}

// accumulateResults dispatches each incoming result to its matching
// position in the accumulated slice, growing the slice as needed.
func (r *ResponseAccumulator) accumulateResults(others []*Result) {
	if len(others) == 0 {
		return
	}

	r.Results = pkgSlices.EnsureIndex(r.Results, len(others)-1)
	for index, other := range others {
		r.accumulateResult(index, other)
	}
}

// accumulateResult merges one chunk's Result at index — assistant message,
// metadata, and tool message in turn.
func (r *ResponseAccumulator) accumulateResult(index int, other *Result) {
	result := r.Results[index]
	if result == nil {
		result = &Result{}
		r.Results[index] = result
	}

	result.AssistantMessage = r.accumulateAssistantMessage(result.AssistantMessage, other.AssistantMessage)
	result.Metadata = r.accumulateResultMetadata(result.Metadata, other.Metadata)
	result.ToolMessage = r.accumulateToolMessage(result.ToolMessage, other.ToolMessage)
}

// accumulateAssistantMessage merges streaming deltas:
//   - Text and Reasoning concatenate ("Hello" + " world" → "Hello world").
//     Reasoning covers visible chain-of-thought (DeepSeek-R1, Anthropic
//     thinking_delta, Gemini thoughts).
//   - Each ToolCall's ID/Name/Arguments concatenate so providers can split
//     long arguments across chunks.
//   - Metadata merges last-write-wins. Provider continuation tokens
//     (signature, redacted thinking) usually arrive in a single final
//     chunk, so overwriting is correct.
func (r *ResponseAccumulator) accumulateAssistantMessage(msg, other *AssistantMessage) *AssistantMessage {
	if other == nil {
		return msg
	}
	if msg == nil {
		msg = &AssistantMessage{}
	}

	msg.Text += other.Text
	msg.Reasoning += other.Reasoning

	if len(other.ToolCalls) > 0 {
		msg.ToolCalls = pkgSlices.EnsureIndex(msg.ToolCalls, len(other.ToolCalls)-1)
		for index, delta := range other.ToolCalls {
			tc := msg.ToolCalls[index]
			if tc == nil {
				tc = &ToolCall{}
				msg.ToolCalls[index] = tc
			}
			tc.ID += delta.ID
			tc.Name += delta.Name
			tc.Arguments += delta.Arguments
		}
	}

	maps.Copy(msg.Meta(), other.Metadata)
	return msg
}

// accumulateToolMessage merges tool execution results. Tool returns are
// atomic (a tool either succeeded with one result or failed) — so the
// strategy is overwrite per index, not concatenate.
func (r *ResponseAccumulator) accumulateToolMessage(msg, other *ToolMessage) *ToolMessage {
	if other == nil {
		return msg
	}
	if msg == nil {
		msg = &ToolMessage{}
	}

	if len(other.ToolReturns) > 0 {
		msg.ToolReturns = pkgSlices.EnsureIndex(msg.ToolReturns, len(other.ToolReturns)-1)
		for index, ret := range other.ToolReturns {
			tr := msg.ToolReturns[index]
			if tr == nil {
				tr = &ToolReturn{}
				msg.ToolReturns[index] = tr
			}
			tr.ID = ret.ID
			tr.Name = ret.Name
			tr.Result = ret.Result
		}
	}

	maps.Copy(msg.Meta(), other.Metadata)
	return msg
}

// accumulateResultMetadata merges per-result metadata. FinishReason
// overwrites (only the final chunk carries it); Extra merges
// last-write-wins.
func (r *ResponseAccumulator) accumulateResultMetadata(meta, other *ResultMetadata) *ResultMetadata {
	if other == nil {
		return meta
	}
	if meta == nil {
		meta = &ResultMetadata{}
	}

	meta.FinishReason = other.FinishReason

	meta.ensureExtra()
	maps.Copy(meta.Extra, other.Extra)
	return meta
}
