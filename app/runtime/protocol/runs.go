package protocol

import (
	"time"
)

// RunStatus is the lifecycle position of a run (§4.1). The three values answer
// what a client has to do next: watch it, answer it, or read it.
type RunStatus string

const (
	// RunStatusRunning — a segment is executing.
	RunStatusRunning RunStatus = "running"
	// RunStatusWaiting — no segment is executing and the run holds open
	// interrupt. It is resumable, and it has NO outcome: "why did it stop" is
	// answered by the interrupts, not by a terminal reason.
	RunStatusWaiting RunStatus = "waiting"
	// RunStatusFinished — no segment, no open interrupt, and a terminal outcome.
	RunStatusFinished RunStatus = "finished"
)

// RunSummary is a run's identity, its place in the run tree, and where it is in
// its lifecycle (§4.2). ID is the STABLE logical run id — a resume continues the
// same run (a new segment), never a new run — so there is no continuation chain
// to carry.
//
// It is the shape a cold read hands back in bulk. Outcome, CreatedAt and
// FinishedAt stay here because they are lifecycle rather than metering: a
// `status:"finished"` a client cannot explain would only send it back for another
// request. What is left out is what grows with the model and the subtree —
// metrics, limits, protocol profile, active segment — and that is [RunRef].
type RunSummary struct {
	ID              string `json:"id"`
	SessionID       string `json:"sessionId"`
	SpawnedByItemID string `json:"spawnedByItemId,omitempty"`
	// ParentRunID and RootRunID are the child edges: direct tree topology, and
	// O(1) routing from any child to the run that owns the subscription. They are
	// all-or-none with SpawnedByItemID — a run either carries all three or is a
	// root. Creating or directly addressing such a child requires
	// features.subagents; durable history retains the edges unchanged.
	ParentRunID string `json:"parentRunId,omitempty"`
	RootRunID   string `json:"rootRunId,omitempty"`
	// Model is the model id this run ran against (Model.id). Normal admission
	// resolves the runtime default before persistence so finished Runs remain
	// self-describing; empty is reserved for imported legacy/unconfigured data.
	Model string `json:"model,omitempty"`
	// Provider is the provider id this run ran against (Provider.id), paired
	// with Model. It is stamped before execution so usage.summary attributes spend by provider without
	// re-deriving the model→provider mapping (which isn't 1:1 across
	// compatible-endpoint providers).
	Provider        string      `json:"provider,omitempty"`
	ReasoningEffort string      `json:"reasoningEffort,omitempty"`
	Status          RunStatus   `json:"status,omitempty"`
	Outcome         *RunOutcome `json:"outcome,omitempty"`
	CreatedAt       time.Time   `json:"createdAt,omitzero"`
	FinishedAt      time.Time   `json:"finishedAt,omitzero"`
}

// RunRef is a run's summary plus the control, metering and protocol facts a
// caller needs in order to drive or account for it (§4.2): what runs.get /
// runs.list and the live segment.started carry.
//
// The summary is EMBEDDED, not copied. encoding/json inlines an embedded struct,
// so one Go definition produces one flat wire shape — there is no second
// declaration of `id` or `status` anywhere that could drift from this one.
type RunRef struct {
	RunSummary
	// ActiveSegmentID is the segment currently executing, and exists exactly while
	// the run is running: a waiting or finished run has none, so a client cannot
	// mistake a segment that already ended for one it can attach to.
	ActiveSegmentID string `json:"activeSegmentId,omitempty"`
	// Metrics is what the run has consumed so far, cumulative over every segment
	// and present in every status. It is not optional: a running run costs money,
	// and a client that can only see spend once a run ends cannot show a budget.
	Metrics RunMetrics `json:"metrics"`
	// ContextTokens is the latest completed model request's prompt footprint.
	// It survives waiting, terminalization, and restart; absence means this Run
	// has not produced an authoritative footprint yet.
	ContextTokens int64 `json:"contextTokens,omitempty"`
	// Limits is the allowance in force for this run, omitted when it runs
	// uncapped. It is the durable execution policy the run was admitted under,
	// not an echo of the request — a resume and a cross-restart recovery report
	// the same caps as the first segment.
	Limits *RunLimits `json:"limits,omitempty"`
	// ProtocolProfile is the protocol contract this run was created under, and it
	// is present in every status: a client that reconnects to a run has to know
	// what the run may publish before it starts folding the stream.
	ProtocolProfile RunProtocolProfile `json:"protocolProfile"`
}

// RunProtocolFeature is the closed set of negotiated features that may appear in
// a RunProtocolProfile for this protocol version. The discovery feature map stays
// open, but a feature that changes authoritative Run shape requires a protocol
// version bump and therefore belongs to this versioned enum.
type RunProtocolFeature string

const (
	RunProtocolFeatureSubagents = RunProtocolFeature(FeatureSubagents)
)

// RunProtocolProfile is the client-observable protocol contract frozen when a run
// was created (§4.2) — not a preference recomputed per request.
//
// Both fields are sets: duplicates are illegal, order carries no meaning, and the
// canonical encoder emits requiredFeatures in lexical order and interruptTypes in
// registry order. Neither is optional, because an empty set is a MEANING: the §8.3
// Minimal Profile — a run that creates no child, publishes no `suspended`, and
// never parks on a human. `null` would say "unknown" about a run whose contract is
// perfectly known.
//
// Its scope is one root run tree, never a session: the next runs.start negotiates
// again, so a minimal client cannot permanently narrow what a session can do.
type RunProtocolProfile struct {
	// RequiredFeatures are the negotiated features that changed what this run
	// publishes (`requiredByRunProtocol` in discovery). A later resume or subscribe
	// whose caller does not declare them is refused, not downgraded — the
	// alternative is a second, quieter event stream for the same run.
	RequiredFeatures []RunProtocolFeature `json:"requiredFeatures"`
	// InterruptTypes are the durable interrupt types this run may produce. The run
	// keeps them for its whole life, so answering an interrupt cannot quietly
	// change what the next segment is allowed to park on.
	InterruptTypes []InterruptType `json:"interruptTypes"`
}

// RunMetrics is how much a run has consumed (§4.2). Total cost reads
// Usage.CostUSD — there is no separate costUsd, which would be a second source
// of one number.
type RunMetrics struct {
	Usage *Usage `json:"usage,omitempty"`
	Steps int    `json:"steps"`
	// ActiveDurationMillis is time spent executing, summed over the run's segments.
	// Waiting on a person is not execution, so a run parked overnight and then
	// answered reports the seconds it worked rather than the hours it existed.
	ActiveDurationMillis int64 `json:"activeDurationMillis"`
}

// RunLimits is the allowance a run may consume before it is stopped (§4.2). An
// omitted field is that dimension uncapped.
type RunLimits struct {
	MaxTotalTokens int64   `json:"maxTotalTokens,omitempty"`
	MaxSteps       int     `json:"maxSteps,omitempty"`
	MaxBudgetUSD   float64 `json:"maxBudgetUsd,omitempty"`
}

// RunOutcomeType discriminates the RunOutcome union (§4.2).
type RunOutcomeType string

const (
	OutcomeCompleted RunOutcomeType = "completed"
	OutcomeTimedOut  RunOutcomeType = "timedOut"
	OutcomeFailed    RunOutcomeType = "failed"
	OutcomeMaxSteps  RunOutcomeType = "maxSteps"
	OutcomeMaxBudget RunOutcomeType = "maxBudget"
	OutcomeCanceled  RunOutcomeType = "canceled"
	OutcomeLost      RunOutcomeType = "lost"
)

// RunOutcome is a tag-discriminated union over why a run STOPPED FOR GOOD
// (§4.2). It answers only that: what the run consumed is RunMetrics, published
// beside it, so neither has to be read through the other.
//
//	completed                → nothing further
//	error                    → Error
//	maxSteps/maxBudget/canceled → optional Detail
//
// An interrupt is deliberately not a member: parking is a status, not a terminal
// reason, and a parked run is resumable. The segment that parked reports it as a
// [SegmentOutcome].
type RunOutcome struct {
	Type RunOutcomeType `json:"type"`
	// Error explains the error terminal and appears on no other. Its own Detail
	// carries the human-readable note (§4.6), which is why Detail below stays
	// absent here rather than repeating it.
	Error *ProblemData `json:"error,omitempty"`
	// Detail is a human-readable note for the non-error terminals
	// (maxSteps / maxBudget / canceled) — lets the client tell "user
	// canceled" from "timed out", show "$X / $Y" for maxBudget, etc. The
	// runs.cancel reason flows here (§4.2).
	Detail string `json:"detail,omitempty"`
}

// SegmentOutcomeType discriminates the SegmentOutcome union (§4.3): every way a
// run can stop for good, plus the two ways a segment can stop while its run
// carries on.
type SegmentOutcomeType string

const (
	// SegmentInterrupt — this segment produced the interrupts it carries, and its
	// run is now waiting for them to be answered.
	SegmentInterrupt SegmentOutcomeType = "interrupt"
	// SegmentSuspended — this segment stopped because another run in its tree
	// interrupted, not because it produced an interrupt itself. It carries no
	// interrupts: they belong to the run that raised them, and copying them here
	// would make one pending item appear twice in the stream.
	SegmentSuspended SegmentOutcomeType = "suspended"

	// The terminals a segment shares with its run. They are CONVERSIONS of the
	// RunOutcomeType constants, not five more string literals: SegmentOutcome
	// contains RunOutcome, so a terminal renamed on one side must be renamed on
	// the other, and a second spelling is exactly how that stops happening.
	SegmentCompleted = SegmentOutcomeType(OutcomeCompleted)
	SegmentTimedOut  = SegmentOutcomeType(OutcomeTimedOut)
	SegmentFailed    = SegmentOutcomeType(OutcomeFailed)
	SegmentMaxSteps  = SegmentOutcomeType(OutcomeMaxSteps)
	SegmentMaxBudget = SegmentOutcomeType(OutcomeMaxBudget)
	SegmentCanceled  = SegmentOutcomeType(OutcomeCanceled)
	SegmentLost      = SegmentOutcomeType(OutcomeLost)
)

// SegmentOutcome is why a SEGMENT stopped (§4.3): either the run stopped for good
// — in which case this is a RunOutcome — or the run is only pausing.
//
//	interrupt                → Interrupts (non-empty)
//	suspended                → nothing further
//	every RunOutcomeType     → as RunOutcome
type SegmentOutcome struct {
	Type SegmentOutcomeType `json:"type"`
	// Error and Detail belong to the terminal tags, and carry exactly what the
	// same-named RunOutcome fields do.
	Error  *ProblemData `json:"error,omitempty"`
	Detail string       `json:"detail,omitempty"`
	// Interrupts is the pending set THIS segment's run raised, and appears only
	// on the interrupt tag.
	Interrupts []Interrupt `json:"interrupts,omitempty"`
}

// StartRunRequest is the runs.start body (API.md §7.1). The session owns cwd,
// available tools, and its Plan, so clients send only the user input and
// explicit execution limits/model selection.
type StartRunRequest struct {
	SessionID string         `json:"sessionId"`
	Input     []ContentBlock `json:"input"`
	// Provider + Model select the model for this run. They are paired: send
	// both to pick a model, or neither to use the runtime's default. Sending
	// one without the other is invalid_params — the provider is explicit,
	// never inferred from the model id. Both are meaningful slugs (no "Id"
	// suffix, mirroring `model` and Model.provider).
	Provider        string            `json:"provider,omitempty"`
	Model           string            `json:"model,omitempty"`
	ReasoningEffort string            `json:"reasoningEffort,omitempty"`
	MaxTotalTokens  int64             `json:"maxTotalTokens,omitempty"`
	MaxSteps        int               `json:"maxSteps,omitempty"`
	MaxBudgetUSD    float64           `json:"maxBudgetUsd,omitempty"`
	Params          *GenerationParams `json:"params,omitempty"`
}

// StartRunResponse is the synchronous result of runs.start.
type StartRunResponse struct {
	RunID string `json:"runId"`
	// SegmentID is the first streamed segment of this Run. The client keys its
	// stream tree and reconnect-replay deduplication on it (§0.3).
	SegmentID string `json:"segmentId"`
	// UserItemID identifies the durable opening userMessage Item. A successful
	// start always creates that Item, so omitting this field would force clients
	// back to ambiguous content matching.
	UserItemID string `json:"userItemId"`
}

// ResumeRunResponse is the synchronous result of runs.resume.
type ResumeRunResponse struct {
	RunID     string `json:"runId"`
	SegmentID string `json:"segmentId"`
	// UserItemID is present exactly when ResumeRunRequest.Input is present. The
	// request and response commit atomically, so this identifies that opening
	// userMessage Item without inventing one for a response-only resume.
	UserItemID *string `json:"userItemId,omitempty"`
}

// GenerationParams is optional LLM generation tuning (API.md §7.1).
type GenerationParams struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int64   `json:"maxTokens,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// CancelRunRequest is the runs.cancel body.
type CancelRunRequest struct {
	RunID  string `json:"runId"`
	Reason string `json:"reason,omitempty"`
}

type CancelRunResponseType string

const (
	CancelRunRoot  CancelRunResponseType = "root"
	CancelRunChild CancelRunResponseType = "child"
)

// CancelRunResponse is the closed result union for runs.cancel. Run is always
// the addressed Finished(canceled) Run. A child result additionally carries
// the root snapshot from the same command boundary so the caller can determine
// whether the surviving tree is running, waiting, or finished without guessing.
type CancelRunResponse struct {
	Type    CancelRunResponseType `json:"type"`
	Run     RunRef                `json:"run"`
	RootRun *RunRef               `json:"rootRun,omitempty"`
}

// SteerRunRequest is the runs.steer body — structured user content to inject
// into the segment the caller believes is executing.
//
// ExpectedSegmentID is required. A steer is the user's instruction about the work
// they are watching; if the run parked and was resumed between typing and
// sending, delivering it anyway would inject that instruction into a
// continuation they never saw. There is no best-effort injection: a mismatch is
// stale_segment, and the client re-reads the run before asking again.
type SteerRunRequest struct {
	RunID             string         `json:"runId"`
	ExpectedSegmentID string         `json:"expectedSegmentId"`
	Input             []ContentBlock `json:"input"`
}

// ListRunsRequest is the runs.list body — the whole durable run history, filtered.
//
// Every filter is independent and omitting one widens the read rather than
// narrowing it: no sessionId pages across every session, no statuses matches every
// lifecycle position. A finished run is not history the read hides — it is the
// answer to what a session did and what it cost.
type ListRunsRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	// Statuses selects lifecycle positions. Omitted means all of them; present
	// means a non-empty set with no repeats, because an empty or duplicated filter
	// is a caller that did not mean what it sent.
	Statuses []RunStatus `json:"statuses,omitempty"`
	// IncludeDescendants adds child runs at any depth to the page. It defaults to
	// false, and an explicit true is a declaration this runtime either honors or
	// refuses with capability_not_negotiated — reading it as false would hand back
	// a page that looks complete and silently is not.
	IncludeDescendants bool `json:"includeDescendants,omitempty"`
	PageQuery
}

// GetRunRequest is the runs.get body. The run id alone identifies the run: a
// caller holding a runId from an event or a page must not have to discover which
// session owns it before it can ask what the run is doing.
type GetRunRequest struct {
	RunID string `json:"runId"`
}

// ListInterruptsRequest is the interrupt.list body. Both filters are optional and
// independent: given together they must both match, and given neither the read
// pages every waiting run tree the runtime holds.
type ListInterruptsRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	// RootRunID narrows to one waiting tree. It must name a root — a child id is
	// refused with run_not_root rather than answered with an empty page, because the
	// set the caller is looking for exists, under its root.
	RootRunID string `json:"rootRunId,omitempty"`
	PageQuery
}

// ResumeRunRequest is the runs.resume body (§6.1). RunID is the stable run to
// continue — its current segment parked with outcome:interrupt.
type ResumeRunRequest struct {
	RunID     string              `json:"runId"`
	Responses []InterruptResponse `json:"responses"`
	// Input is optional user input to add while answering. It exists because
	// "approve, and also do this differently" was otherwise two calls — resume, then
	// steer — with a race in between where the model could finish the tool round
	// before the instruction arrived. Given here, the user Item commits in the SAME
	// transaction as the continuation, so either both landed or neither did.
	//
	// When it is present the response carries userItemId, and when it is absent the
	// response must not: there is no item to name.
	Input []ContentBlock `json:"input,omitempty"`
}

// SubscribeRunRequest identifies the run AND the segment to rebind to the
// caller.
//
// SegmentID is required. A subscription that named only the run would attach to
// whichever segment happened to be live, and a client that had been folding an
// earlier one would silently continue into a different execution — its own
// reconnect turning into an undetectable state corruption. Naming the segment
// makes the mismatch a stale_segment refusal, and the client re-reads the run to
// find out what actually happened.
type SubscribeRunRequest struct {
	RunID     string `json:"runId"`
	SegmentID string `json:"segmentId"`
}

// SubscribeRunResponse acknowledges an attached stream.
//
// It is not a StartRunResponse: subscribing opens no Segment, so there is no
// userItemId, and an ack that declared one would publish a field nothing on this
// path can write.
type SubscribeRunResponse struct {
	RunID     string `json:"runId"`
	SegmentID string `json:"segmentId"`
	// HeadEventID is the stream's position at the instant the subscription was
	// established, absent when the stream has published nothing yet.
	//
	// It has exactly two legal uses: comparing it for EQUALITY against an eventId
	// to drop a duplicate, and storing it unchanged to send back as the next
	// reconnect's cursor. It is not a watermark — clients must not parse it, order
	// it, compare it for magnitude, or derive a sequence from it. The value is
	// opaque precisely so that the runtime can change what it encodes without
	// breaking a client that only ever handed it back.
	HeadEventID string `json:"headEventId,omitempty"`
}

// InterruptResponseType discriminates a client's answer to an interrupt
// (API.md §6.1). "answer" responds to a "question" interrupt.
type InterruptResponseType string

const (
	InterruptResponseApproval InterruptResponseType = "approval"
	InterruptResponseAnswer   InterruptResponseType = "answer"
)

// ApprovalDecision is the verdict on an approval interrupt (API.md §6.1).
type ApprovalDecision string

const (
	ApprovalApprove ApprovalDecision = "approve"
	ApprovalDeny    ApprovalDecision = "deny"
)

// InterruptResponse answers one open interrupt, keyed by itemId (API.md §6.1).
// Response is a tag-discriminated union (Type):
//
//	approval → Decision, EditedArgs, Reason
//	answer   → Answers
type InterruptResponse struct {
	ItemID   string                 `json:"itemId"`
	Response InterruptResponseValue `json:"response"`
}

// InterruptResponseValue is the discriminated response payload.
type InterruptResponseValue struct {
	Type       InterruptResponseType `json:"type"`                 // see InterruptResponseType
	Decision   ApprovalDecision      `json:"decision,omitempty"`   // approval: see ApprovalDecision
	Remember   *RememberScope        `json:"remember,omitempty"`   // approval: keep this decision (AUX_API §6)
	EditedArgs map[string]any        `json:"editedArgs,omitempty"` // approval: one-shot arg override
	Reason     string                `json:"reason,omitempty"`     // approval (deny rationale)
	Answers    [][]string            `json:"answers,omitempty"`    // answer: one values array per Question.fields entry, in the same order
}

// RememberScopeKind is the persistence scope of a remembered approval (AUX_API
// §6): the decision is stored as a rule reaching one session, one project
// directory, or everywhere. All three persist (sqlite-backed) and auto-resolve
// matching future calls.
type RememberScopeKind string

const (
	RememberSession RememberScopeKind = "session"
	RememberProject RememberScopeKind = "project"
	RememberGlobal  RememberScopeKind = "global"
)

// RememberScope is the standing-decision directive on an approval response
// (AUX_API §6). When present the runtime persists the approve/deny decision as
// a fine-grained rule so matching future calls skip the prompt. The rule is
// keyed by tool NAME + the call's per-tool subject (a shell command, an edited
// file's path) at the chosen Scope (session / project / global). editedArgs
// stays one-shot regardless: a remembered rule matches by subject, never by a
// one-off arg rewrite.
type RememberScope struct {
	Scope RememberScopeKind `json:"scope"` // see RememberScopeKind
}

// InterruptType discriminates a pending interrupt (API.md §4.8): a tool awaiting
// approval or a question awaiting an answer.
type InterruptType string

const (
	InterruptApproval InterruptType = "approval"
	InterruptQuestion InterruptType = "question"
)

// ApprovalRisk is the coarse severity shown on an approval prompt.
type ApprovalRisk string

const (
	ApprovalRiskLow    ApprovalRisk = "low"
	ApprovalRiskMedium ApprovalRisk = "medium"
	ApprovalRiskHigh   ApprovalRisk = "high"
)

// InterruptPayload is the self-contained data for one [Interrupt]. Type
// determines the legal fields:
//
//	approval -> Tool, optional Risk and Reason, Rememberable
//	question -> Question
//
// The pointer fields retain the wire distinction between an absent member and
// a member whose value happens to be empty while avoiding an open-ended map at
// the protocol boundary.
type InterruptPayload struct {
	Tool         *ToolInvocation `json:"tool,omitempty"`
	Risk         ApprovalRisk    `json:"risk,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Rememberable bool            `json:"rememberable,omitempty"`
	Question     *Question       `json:"question,omitempty"`
}

// Interrupt is one pending HITL item (§4.8). ItemID is the correlation key — the
// toolCall or question item awaiting resolution — and RunID is the run that raised
// it, which is not necessarily the run that owns the set: one set can hold
// interrupts raised anywhere in a run tree, and each is answered in the context of
// the run that asked.
type Interrupt struct {
	ItemID  string            `json:"itemId"`
	RunID   string            `json:"runId"`
	Type    InterruptType     `json:"type"` // see InterruptType
	Payload *InterruptPayload `json:"payload,omitempty"`
}

// PendingInterruptSet is everything one waiting run tree needs answered, and the
// unit both the read and the resume work in (§4.8 / §6.2).
//
// It is a SET, not a list of independent items: runs.resume validates and consumes
// all of it in one transaction, so a page never splits one — half a set is a resume
// that cannot be attempted.
//
// RootRunID is the run to resume: the root that owns the tree. It is deliberately
// not called runId, because each Interrupt carries the run that RAISED it and the
// two answer different questions.
type PendingInterruptSet struct {
	RootRunID  string      `json:"rootRunId"`
	SessionID  string      `json:"sessionId"`
	Interrupts []Interrupt `json:"interrupts"`
	CreatedAt  time.Time   `json:"createdAt"`
}
