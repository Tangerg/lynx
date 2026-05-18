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
	r.accumulateResult(chunk.Result)
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

// accumulateResult merges one chunk's Result into the accumulated state —
// assistant message, metadata, and tool message in turn.
func (r *ResponseAccumulator) accumulateResult(other *Result) {
	if other == nil {
		return
	}
	if r.Result == nil {
		r.Result = &Result{}
	}

	r.Result.AssistantMessage = r.accumulateAssistantMessage(r.Result.AssistantMessage, other.AssistantMessage)
	r.Result.Metadata = r.accumulateResultMetadata(r.Result.Metadata, other.Metadata)
	r.Result.ToolMessage = r.accumulateToolMessage(r.Result.ToolMessage, other.ToolMessage)
}

// accumulateAssistantMessage merges streaming deltas at the Part level.
// Each incoming AssistantMessage carries one or more delta Parts; the
// part-level Accumulator handles same-type runs (TextPart text
// concatenates; ToolCallPart args concatenate when the ID matches) and
// flushes on type or identity changes. Metadata merges last-write-wins
// at the message level.
func (r *ResponseAccumulator) accumulateAssistantMessage(msg, other *AssistantMessage) *AssistantMessage {
	if other == nil {
		return msg
	}
	if msg == nil {
		msg = &AssistantMessage{}
	}

	if len(other.Parts) > 0 {
		// Seed a part-level Accumulator with the parts gathered so far,
		// feed the new deltas through it, and rebuild Parts. Re-seeding
		// keeps the contract that already-flushed parts remain stable
		// (a finalized TextPart at index 3 does not grow when a NEW
		// trailing TextPart arrives at index 4).
		var acc Accumulator
		acc.AddAll(msg.Parts)
		acc.AddAll(other.Parts)
		msg.Parts = acc.Build()
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
