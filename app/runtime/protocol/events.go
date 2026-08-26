package protocol

import "time"

// RunEvent is the params of the notifications.run.event notification —
// the single downstream stream carrying segment / item / Plan events
// (API.md §5). RunID is the stable logical run; SegmentID is the streamed
// segment the event belongs to (§0.3) — a client scopes its stream tree +
// reconnect-replay dedup to it. eventId is monotonic within one segment stream.
//
// There is no per-frame reliability flag. Authoritativeness and replayability
// are protocol facts owned by the event type.
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
	StreamPlanUpdated     StreamEventType = "plan.updated"
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
//	plan.updated    → Plan
type StreamEvent struct {
	Type StreamEventType `json:"type"`

	Run      *RunRef         `json:"run,omitempty"`
	Progress *RunProgress    `json:"progress,omitempty"`
	Outcome  *SegmentOutcome `json:"outcome,omitempty"`
	// Metrics rides every segment.finished, terminal or not: a client reads what
	// the run consumed from one field instead of looking for it in whichever
	// branch of the outcome happens to carry it.
	Metrics *RunMetrics `json:"metrics,omitempty"`
	Item    *Item       `json:"item,omitempty"`
	ItemID  string      `json:"itemId,omitempty"`
	Delta   *ItemDelta  `json:"delta,omitempty"`
	Plan    *Plan       `json:"plan,omitempty"`
}

// Authoritative reports whether the event itself is a fact a client may fold.
// It is deliberately separate from [StreamEvent.Replayable]: the current core
// event set happens to give every authoritative frame a replay window, but
// neither concept defines the other.
func (s StreamEvent) Authoritative() bool {
	switch s.Type {
	case StreamSegmentStarted, StreamSegmentFinished,
		StreamItemStarted, StreamItemCompleted, StreamPlanUpdated:
		return true
	default:
		return false
	}
}

// Replayable reports whether the Runtime-instance-local segment journal retains this
// event and whether its HTTP frame receives an SSE id. Unknown events fail
// closed and never enter the replay window.
func (s StreamEvent) Replayable() bool {
	switch s.Type {
	case StreamSegmentStarted, StreamSegmentFinished,
		StreamItemStarted, StreamItemCompleted, StreamPlanUpdated:
		return true
	default:
		return false
	}
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

// Plan is the Session's persisted latest Plan. A root Run publishes it through
// plan.updated, and plan.get returns the same shape, so live and cold recovery cannot
// describe the checklist differently (§5.2 / §5.3).
//
// Revision is the projection's own monotonic counter, assigned by the replacement
// that produced it. Zero means nothing has ever been written — the empty list a
// session starts with — and it is what tells an older snapshot from a newer one when
// the contents alone cannot: the list is replaced wholesale, so it can shrink.
type Plan struct {
	SessionID string     `json:"sessionId"`
	Revision  uint64     `json:"revision"`
	Steps     []PlanStep `json:"steps"`
	// UpdatedAt is absent exactly while Revision is 0: nothing was written, so there
	// is no time at which it was.
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
}

// GetPlanRequest is the plan.get body — the cold read for the Plan projection.
type GetPlanRequest struct {
	SessionID string `json:"sessionId"`
}

// PlanStep is one Step of the session [Plan].
// The Plan is replaced whole each set_plan, so ID is positional — a stable key
// within a snapshot, not a durable identity. Status is
// "pending" | "in_progress" | "completed".
type PlanStep struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
}

// PlanStatus is one Step's execution state.
type PlanStatus string

const (
	PlanStatusPending    PlanStatus = "pending"
	PlanStatusInProgress PlanStatus = "in_progress"
	PlanStatusCompleted  PlanStatus = "completed"
)

// ItemDeltaType discriminates the ItemDelta union (API.md §5.1).
type ItemDeltaType string

const (
	DeltaContent       ItemDeltaType = "content"
	DeltaReasoning     ItemDeltaType = "reasoning"
	DeltaToolArguments ItemDeltaType = "toolArguments"
	DeltaToolOutput    ItemDeltaType = "toolOutput"
)

// ItemDelta is a tag-discriminated union over incremental updates
// (API.md §5.1). All delta events are non-authoritative and non-replayable.
//
//	content       → Index, Text
//	reasoning     → Text
//	toolArguments → ArgumentsTextDelta (partial JSON text; client repairs)
//	toolOutput    → Text
type ItemDelta struct {
	Type ItemDeltaType `json:"type"`

	Index              *int   `json:"index,omitempty"`
	Text               string `json:"text,omitempty"`
	ArgumentsTextDelta string `json:"argumentsTextDelta,omitempty"`
}
