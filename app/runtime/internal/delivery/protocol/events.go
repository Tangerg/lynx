package protocol

import "time"

// RunEvent is the params of the notifications.run.event notification —
// the single downstream stream carrying run / item / state events
// (API.md §5). RunID is the stable logical run; SegmentID is the streamed
// segment the event belongs to (§0.3) — a client scopes its stream tree +
// reconnect-replay dedup to it. eventId is monotonic within one segment stream.
//
// There is NO per-frame `durable` bool (S4): durability is a pure function
// of the event type (StreamEvent.IsDurable), so a redundant field that could
// drift — claiming e.g. item.completed with durable:false — is removed. The
// only event whose durability isn't derivable from its type is `custom`,
// which carries its own optional flag on StreamEvent.Durable.
type RunEvent struct {
	RunID     string      `json:"runId"`
	SegmentID string      `json:"segmentId"` // seg_…
	EventID   string      `json:"eventId"`   // evt_…
	Timestamp time.Time   `json:"timestamp"` // ISO-8601 (time.Time marshals to RFC3339)
	Event     StreamEvent `json:"event"`
}

// Durable, Terminal, and Cursor let a RunEvent satisfy the application/runs
// Journal's Event interface (buffer / fan-out / replay after a cursor) without
// the Journal knowing anything wire-specific. EventID is fixed-width
// zero-padded, so the Journal's lexical cursor comparison agrees with numeric.
func (e RunEvent) Durable() bool  { return e.Event.IsDurable() }
func (e RunEvent) Terminal() bool { return e.Event.Type == StreamSegmentFinished }
func (e RunEvent) Cursor() string { return e.EventID }

// StreamEventType discriminates the StreamEvent union (API.md §5).
type StreamEventType string

const (
	StreamSegmentStarted  StreamEventType = "segment.started"
	StreamSegmentProgress StreamEventType = "segment.progress"
	StreamSegmentFinished StreamEventType = "segment.finished"
	StreamItemStarted     StreamEventType = "item.started"
	StreamItemDelta       StreamEventType = "item.delta"
	StreamItemCompleted   StreamEventType = "item.completed"
	StreamStateSnapshot   StreamEventType = "state.snapshot"
	StreamStateDelta      StreamEventType = "state.delta"
	StreamCustom          StreamEventType = "custom"
)

// StreamEvent is a tag-discriminated union over downstream events
// (API.md §5). Type selects which optional fields apply.
//
//	segment.started     → Run
//	segment.progress    → Progress
//	segment.finished    → Outcome
//	item.started    → Item
//	item.delta      → ItemID, Delta
//	item.completed  → Item
//	state.snapshot  → State
//	state.delta     → Patch
//	custom          → Name, Payload, Durable?
type StreamEvent struct {
	Type StreamEventType `json:"type"`

	Run      *RunRef        `json:"run,omitempty"`
	Progress *RunProgress   `json:"progress,omitempty"`
	Outcome  *RunOutcome    `json:"outcome,omitempty"`
	Item     *Item          `json:"item,omitempty"`
	ItemID   string         `json:"itemId,omitempty"`
	Delta    *ItemDelta     `json:"delta,omitempty"`
	State    map[string]any `json:"state,omitempty"`
	Patch    JsonPatch      `json:"patch,omitempty"`
	Name     string         `json:"name,omitempty"`    // custom
	Payload  any            `json:"payload,omitempty"` // custom
	Durable  *bool          `json:"durable,omitempty"` // custom only — its self-declared durability (default false)
}

// IsDurable reports whether a stream event is durable (authoritative /
// replayable, retained for replay + persisted) per the §5.2 derivation
// table. Durability is a pure function of the event type for every
// first-party event; only `custom` carries its own flag (StreamEvent.Durable,
// default false). This is the single source for the durable/ephemeral split —
// the hub's replay buffer and the SSE `id:` gate both read it, so neither
// derives durability independently.
func (se StreamEvent) IsDurable() bool {
	switch se.Type {
	case StreamItemDelta, StreamStateDelta, StreamSegmentProgress:
		return false
	case StreamCustom:
		return se.Durable != nil && *se.Durable
	default:
		return true
	}
}

// RunProgress is the mid-run progress preview carried by a segment.progress
// event (API.md §5). Ephemeral — its terminal values land on
// segment.finished.result (usage incl. costUsd / steps).
type RunProgress struct {
	Step *int `json:"step,omitempty"`
	// MaxSteps would complete a "step N of M" counter, but no producer sets it
	// yet: Step counts tool CALLS while the run cap (StartRun.maxSteps) counts
	// tool ROUNDS, so emitting M here would mismatch N until the units align.
	// The cap is still enforced — it terminates the run (outcome:maxSteps)
	// rather than streaming a live countdown. Staged, not an oversight.
	MaxSteps *int   `json:"maxSteps,omitempty"`
	Usage    *Usage `json:"usage,omitempty"`
	// ContextTokens is the latest round's prompt-token count — the live
	// context-window occupancy (how full the window is right now), distinct from
	// the cumulative-over-rounds Usage.inputTokens (which only grows). Pair it
	// with the served model's contextWindow (models.list) for an occupancy gauge;
	// it drops after a compaction. Ephemeral, like the rest of RunProgress.
	ContextTokens *int64 `json:"contextTokens,omitempty"`
	Activity      string `json:"activity,omitempty"` // human-readable current action
}

// TodoSnapshot is one entry of the model's task list, projected to
// state.snapshot under the "todos" key (AUX_API §3.x). The list is replaced
// whole each todo_write, so ID is positional — a stable key within a snapshot,
// not a durable identity. Status is "pending" | "in_progress" | "completed".
type TodoSnapshot struct {
	ID            string `json:"id"`
	Text          string `json:"text"`
	Status        string `json:"status"`
	BlockedReason string `json:"blockedReason,omitempty"`
	NextAction    string `json:"nextAction,omitempty"`
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

// PatchOp is one RFC 6902 operation verb (API.md §5).
type PatchOp string

const (
	PatchOpAdd     PatchOp = "add"
	PatchOpRemove  PatchOp = "remove"
	PatchOpReplace PatchOp = "replace"
	PatchOpMove    PatchOp = "move"
	PatchOpCopy    PatchOp = "copy"
	PatchOpTest    PatchOp = "test"
)

// JsonPatchOp is one RFC 6902 operation.
type JsonPatchOp struct {
	Op    PatchOp `json:"op"` // see PatchOp
	Path  string  `json:"path"`
	Value any     `json:"value,omitempty"`
	From  string  `json:"from,omitempty"`
}
