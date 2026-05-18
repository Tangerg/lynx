package chat

// Accumulator merges streaming [OutputPart] deltas into the final
// ordered list. Same-type adjacent deltas are merged in-place via
// each part's appendDelta; type changes (or identity changes for tool
// calls) flush the in-flight part and start a new one.
//
// The implementation is completely type-agnostic: it never does a
// concrete type switch on OutputPart. Adding new part kinds in the
// future requires zero change here — they just need to satisfy
// [OutputPart] and decide their own appendDelta semantics.
//
// Accumulator is NOT safe for concurrent use; instantiate one per
// stream.
type Accumulator struct {
	parts   []OutputPart // finalized parts
	current OutputPart   // in-flight; nil between flushes
}

// Add applies one part delta. Typical use: drain
// [Response.Result.AssistantMessage.Parts] from each streaming
// Response into the accumulator one delta at a time.
//
// Nil deltas are ignored.
func (a *Accumulator) Add(delta OutputPart) {
	if delta == nil {
		return
	}
	if a.current == nil {
		a.current = delta
		return
	}
	if a.current.appendDelta(delta) {
		return
	}
	a.parts = append(a.parts, a.current)
	a.current = delta
}

// AddAll is a convenience wrapper around [Accumulator.Add] for batch
// ingestion.
func (a *Accumulator) AddAll(deltas []OutputPart) {
	for _, d := range deltas {
		a.Add(d)
	}
}

// Build flushes the in-flight part (if any) and returns the final
// slice. Safe to call multiple times: subsequent calls return the
// same slice without re-flushing.
func (a *Accumulator) Build() []OutputPart {
	if a.current != nil {
		a.parts = append(a.parts, a.current)
		a.current = nil
	}
	return a.parts
}

// Reset clears the accumulator so it can be reused.
func (a *Accumulator) Reset() {
	a.parts = nil
	a.current = nil
}
