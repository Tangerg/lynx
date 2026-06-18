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
	parts partAccumulator
}

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

// accumulateAssistantMessage merges streaming deltas at the Part level
// by feeding them through r.parts — a stateful part-level accumulator
// that owns the running Parts slice. Metadata merges last-write-wins
// at the message level.
func (r *ResponseAccumulator) accumulateAssistantMessage(msg, other *AssistantMessage) *AssistantMessage {
	if other == nil {
		return msg
	}
	if msg == nil {
		msg = &AssistantMessage{}
	}

	if len(other.Parts) > 0 {
		r.parts.addAll(other.Parts)
		msg.Parts = r.parts.parts
	}

	maps.Copy(msg.Meta(), other.Metadata)
	return msg
}

// partAccumulator merges streaming [OutputPart] deltas into one
// ordered slice. Same-type adjacent deltas are merged in-place via
// each part's appendDelta; type changes (or identity changes for
// tool calls) start a new entry. Type-agnostic — never type-switches
// on concrete parts; adding new part kinds requires no change here.
//
// Implementation note: the in-flight part lives at parts[len-1] —
// no separate "current" slot — so the accumulated slice IS the
// authoritative Parts view at all times.
type partAccumulator struct {
	parts []OutputPart
}

// add applies one part delta. Nil deltas are ignored.
//
// A new logical part is adopted as a clone, never the caller's delta:
// later same-type deltas merge in-place into the running part, and we
// must not mutate a part the caller still holds. This is load-bearing
// when one chunk stream feeds two accumulators (e.g. the tool-loop and
// memory stream middlewares) — without the clone the inner accumulator's
// in-place merge would corrupt the part the outer one accumulates,
// double-counting every delta.
func (a *partAccumulator) add(delta OutputPart) {
	if delta == nil {
		return
	}
	if n := len(a.parts); n > 0 && a.parts[n-1].appendDelta(delta) {
		return
	}
	a.parts = append(a.parts, delta.clone())
}

func (a *partAccumulator) addAll(deltas []OutputPart) {
	for _, d := range deltas {
		a.add(d)
	}
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
