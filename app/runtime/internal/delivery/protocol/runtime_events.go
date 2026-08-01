package protocol

// RuntimeEventType discriminates the [RuntimeEvent] union (§7.3): the nine change
// signals a client can subscribe to, plus the one frame that is not a change —
// `resync`, which says the stream lost its place.
//
// Nine of the ten values are also the subscribable topics. They are ONE set of
// strings, written here and referenced by [RuntimeTopic], because the alternative is
// a topic table and an event table that can disagree about a name.
type RuntimeEventType string

const (
	// RuntimeFilesChanged — files under a registered watch, or files an agent tool
	// wrote, changed on disk.
	RuntimeFilesChanged RuntimeEventType = "files.changed"
	// RuntimeSkillsChanged — the discovered skill set changed.
	RuntimeSkillsChanged RuntimeEventType = "skills.changed"
	// RuntimeMCPChanged — an MCP server's registration or connection changed.
	RuntimeMCPChanged RuntimeEventType = "mcp.changed"
	// RuntimeSchedulesChanged — a schedule was created, edited, deleted or fired.
	RuntimeSchedulesChanged RuntimeEventType = "schedules.changed"
	// RuntimeSessionsChanged — a session was created, renamed, deleted or its
	// lifecycle moved.
	RuntimeSessionsChanged RuntimeEventType = "sessions.changed"
	// RuntimeRunsChanged — a run's lifecycle position changed.
	RuntimeRunsChanged RuntimeEventType = "runs.changed"
	// RuntimeStateChanged — a durable state projection changed, for a client that is
	// NOT following the run that wrote it. The run's own stream carries the snapshot
	// itself (§5.6); this only says "read it again".
	RuntimeStateChanged RuntimeEventType = "state.changed"
	// RuntimeGoalsChanged — a session's goal changed.
	RuntimeGoalsChanged RuntimeEventType = "goals.changed"
	// RuntimeInterruptsChanged — a waiting set opened, was answered, or was canceled.
	RuntimeInterruptsChanged RuntimeEventType = "interrupts.changed"
	// RuntimeResync — the stream could not keep every change, so what a client holds
	// may be stale. It names the topics and watches affected; the remedy is to read
	// them again.
	RuntimeResync RuntimeEventType = "resync"
)

// RuntimeTopic is what a subscription asks for. Its values are exactly the change
// signals — `resync` is not subscribable, because a client does not ask to be told it
// fell behind.
type RuntimeTopic string

const (
	TopicFilesChanged      = RuntimeTopic(RuntimeFilesChanged)
	TopicSkillsChanged     = RuntimeTopic(RuntimeSkillsChanged)
	TopicMCPChanged        = RuntimeTopic(RuntimeMCPChanged)
	TopicSchedulesChanged  = RuntimeTopic(RuntimeSchedulesChanged)
	TopicSessionsChanged   = RuntimeTopic(RuntimeSessionsChanged)
	TopicRunsChanged       = RuntimeTopic(RuntimeRunsChanged)
	TopicStateChanged      = RuntimeTopic(RuntimeStateChanged)
	TopicGoalsChanged      = RuntimeTopic(RuntimeGoalsChanged)
	TopicInterruptsChanged = RuntimeTopic(RuntimeInterruptsChanged)
)

// RuntimeTopics is the closed subscribable set, in declaration order. It is the one
// list: discovery advertises from it, the subscribe request is validated against it,
// and the published enum is generated from it.
var RuntimeTopics = []RuntimeTopic{
	TopicFilesChanged, TopicSkillsChanged, TopicMCPChanged, TopicSchedulesChanged,
	TopicSessionsChanged, TopicRunsChanged, TopicStateChanged, TopicGoalsChanged,
	TopicInterruptsChanged,
}

// RuntimeSubscriptionLimits caps one subscription. Both are fixed rather than
// configurable: they exist to bound one client's fan-out, not to be tuned.
const (
	MaxSubscriptionTopics  = 32
	MaxSubscriptionWatches = 32
)

// RuntimeSubscribeRequest is the runtime.subscribe body (§7.2). Topics is required
// and non-empty: there is no wildcard and no "subscribe to everything", because a
// client that has not said what it can fold cannot be sent everything.
type RuntimeSubscribeRequest struct {
	Topics []RuntimeTopic `json:"topics"`
	// Watches register file-watch roots. Legal only alongside files.changed — the
	// other topics are global, so a watch would narrow nothing.
	Watches []WatchSpec `json:"watches,omitempty"`
}

// WatchSpec is one file-watch registration. WatchID is client-chosen and echoed on
// every files.changed it produces; Cwd defaults to the serve directory.
type WatchSpec struct {
	WatchID string `json:"watchId"`
	Cwd     string `json:"cwd,omitempty"`
}

// RuntimeSubscribeResponse is the (empty) streaming ack — the first frame of the
// stream, mirroring StartRunResponse's role for runs.
type RuntimeSubscribeResponse struct{}

// RuntimeEventNotification is the params carried by
// notifications.runtime.event. The wrapper is part of the wire contract: unlike
// run events, runtime events live under an event member so the notification can
// grow envelope metadata without changing the event union itself.
type RuntimeEventNotification struct {
	Event RuntimeEvent `json:"event"`
}

// RuntimeEvent is one change signal (§7.3): a flat tag-discriminated struct whose
// optional fields say WHICH resources moved.
//
// Every variant is an invalidation, not a payload. It carries ids so a client can
// narrow what it refetches, and nothing else — a status or a count here would be a
// second source of truth for something a query already answers, and the two would
// drift the moment one frame was dropped.
type RuntimeEvent struct {
	Type RuntimeEventType `json:"type"`
	// Sequence is monotonic per subscription, so a client can tell it missed frames
	// even when it cannot tell which.
	Sequence uint64 `json:"sequence"`

	// files.changed
	WatchID string   `json:"watchId,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	// skills.changed
	Names []string `json:"names,omitempty"`
	// mcp.changed
	ServerIDs []string `json:"serverIds,omitempty"`
	// schedules.changed
	ScheduleIDs []string `json:"scheduleIds,omitempty"`
	// sessions.changed / runs.changed / state.changed / goals.changed /
	// interrupts.changed — the resources to read again.
	SessionIDs []string `json:"sessionIds,omitempty"`
	RunIDs     []string `json:"runIds,omitempty"`
	// state.changed names which projection moved, so a client refetches that key
	// rather than every key it holds.
	Key StateSnapshotType `json:"key,omitempty"`
	// resync
	Topics   []RuntimeTopic `json:"topics,omitempty"`
	WatchIDs []string       `json:"watchIds,omitempty"`
}
