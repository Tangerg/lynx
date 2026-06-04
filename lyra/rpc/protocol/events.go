package protocol

import "time"

// RunEvent is the params of the notifications.run.event notification —
// the single downstream stream carrying run / item / state events
// (API.md §5). eventId is monotonic within one root run stream.
type RunEvent struct {
	RunID     string      `json:"runId"`
	EventID   string      `json:"eventId"`   // evt_…
	Timestamp time.Time   `json:"timestamp"` // ISO-8601 (time.Time marshals to RFC3339)
	Durable   bool        `json:"durable"`   // true=authoritative/listable; false=ephemeral delta
	Event     StreamEvent `json:"event"`
}

// StreamEventType discriminates the StreamEvent union (API.md §5).
type StreamEventType string

const (
	StreamRunStarted    StreamEventType = "run.started"
	StreamRunFinished   StreamEventType = "run.finished"
	StreamItemStarted   StreamEventType = "item.started"
	StreamItemDelta     StreamEventType = "item.delta"
	StreamItemCompleted StreamEventType = "item.completed"
	StreamStateSnapshot StreamEventType = "state.snapshot"
	StreamStateDelta    StreamEventType = "state.delta"
	StreamCustom        StreamEventType = "custom"
)

// StreamEvent is a tag-discriminated union over downstream events
// (API.md §5). Type selects which optional fields apply.
//
//	run.started     → Run
//	run.finished    → Outcome
//	item.started    → Item
//	item.delta      → ItemID, Delta
//	item.completed  → Item
//	state.snapshot  → State
//	state.delta     → Patch
//	custom          → Name, Payload
type StreamEvent struct {
	Type StreamEventType `json:"type"`

	Run     *RunRef        `json:"run,omitempty"`
	Outcome *RunOutcome    `json:"outcome,omitempty"`
	Item    *Item          `json:"item,omitempty"`
	ItemID  string         `json:"itemId,omitempty"`
	Delta   *ItemDelta     `json:"delta,omitempty"`
	State   map[string]any `json:"state,omitempty"`
	Patch   JsonPatch      `json:"patch,omitempty"`
	Name    string         `json:"name,omitempty"`    // custom
	Payload any            `json:"payload,omitempty"` // custom
}

// ItemDeltaType discriminates the ItemDelta union (API.md §5.1).
type ItemDeltaType string

const (
	DeltaContent       ItemDeltaType = "content"
	DeltaReasoning     ItemDeltaType = "reasoning"
	DeltaToolArguments ItemDeltaType = "toolArguments"
	DeltaToolOutput    ItemDeltaType = "toolOutput"
	DeltaPlan          ItemDeltaType = "plan"
)

// ItemDelta is a tag-discriminated union over incremental updates
// (API.md §5.1). All delta events are durable=false.
//
//	content       → Index, Text
//	reasoning     → Text
//	toolArguments → ArgumentsTextDelta (partial JSON text; client repairs)
//	toolOutput    → Text
//	plan          → Steps (current full snapshot)
type ItemDelta struct {
	Type ItemDeltaType `json:"type"`

	Index              *int       `json:"index,omitempty"`
	Text               string     `json:"text,omitempty"`
	ArgumentsTextDelta string     `json:"argumentsTextDelta,omitempty"`
	Steps              []PlanStep `json:"steps,omitempty"`
}

// JsonPatch is an RFC 6902 patch (API.md §5).
type JsonPatch []JsonPatchOp

// JsonPatchOp is one RFC 6902 operation.
type JsonPatchOp struct {
	Op    string `json:"op"` // "add"|"remove"|"replace"|"move"|"copy"|"test"
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
	From  string `json:"from,omitempty"`
}
