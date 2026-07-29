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
	StreamCustom          StreamEventType = "custom"
)

// StreamEvent is a tag-discriminated union over downstream events
// (API.md §5). Type selects which optional fields apply.
//
//	segment.started     → Run
//	segment.progress    → Progress
//	segment.finished    → Outcome, Metrics
//	item.started    → Item
//	item.delta      → ItemID, Delta
//	item.completed  → Item
//	state.snapshot  → State
//	custom          → Name, Payload, Durable?
type StreamEvent struct {
	Type StreamEventType `json:"type"`

	Run      *RunRef         `json:"run,omitempty"`
	Progress *RunProgress    `json:"progress,omitempty"`
	Outcome  *SegmentOutcome `json:"outcome,omitempty"`
	// Metrics rides every segment.finished, terminal or not: a client reads what
	// the run consumed from one field instead of looking for it in whichever
	// branch of the outcome happens to carry it.
	Metrics *RunMetrics    `json:"metrics,omitempty"`
	Item    *Item          `json:"item,omitempty"`
	ItemID  string         `json:"itemId,omitempty"`
	Delta   *ItemDelta     `json:"delta,omitempty"`
	State   map[string]any `json:"state,omitempty"`
	Name    string         `json:"name,omitempty"`    // custom
	Payload any            `json:"payload,omitempty"` // custom
	Durable *bool          `json:"durable,omitempty"` // custom only — its self-declared durability (default false)
}

// IsDurable reports whether a stream event is durable (authoritative /
// replayable, retained for replay + persisted) per the §5.2 derivation
// table. Durability is a pure function of the event type for every
// first-party event; only `custom` carries its own flag (StreamEvent.Durable,
// default false). This is the single source for the durable/ephemeral split —
// the hub's replay buffer and the SSE `id:` gate both read it, so neither
// derives durability independently.
func (se StreamEvent) IsDurable() bool {
	if se.Type == StreamCustom {
		return se.Durable != nil && *se.Durable
	}
	return !se.Type.AlwaysEphemeral()
}

// AlwaysEphemeral reports whether every event of this type is ephemeral — the
// half of the §5.2 split the type alone decides. `custom` is deliberately not
// among them: it declares its durability per event, so a client may not opt out
// of the type wholesale, and only the types answering unconditionally may appear
// in [ClientCapabilities.ExcludedEphemeralEvents].
func (t StreamEventType) AlwaysEphemeral() bool {
	return t == StreamItemDelta || t == StreamSegmentProgress
}

// RunProgress is the mid-run progress preview carried by a segment.progress
// event (API.md §5). Ephemeral — it previews the same run-cumulative figures
// that land authoritatively on segment.finished.metrics, so it may run briefly
// ahead of them but never contradicts them.
type RunProgress struct {
	Step  *int   `json:"step,omitempty"`
	Usage *Usage `json:"usage,omitempty"`
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
	ID            string     `json:"id"`
	Text          string     `json:"text"`
	Status        TodoStatus `json:"status"`
	BlockedReason string     `json:"blockedReason,omitempty"`
	NextAction    string     `json:"nextAction,omitempty"`
}

// TodoStatus is the model-maintained checklist lifecycle.
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
)

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
