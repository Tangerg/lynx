package protocol

import (
	"context"
	"time"
)

// Runs is the runs.* method group (API.md §7.3). HITL uses the R model:
// a run finishes with an interrupt outcome; the client resumes via a
// continuation run.
type Runs interface {
	// StartRun starts a new run and opens its event stream. Returns the
	// runId synchronously; events flow out via notifications.run.event.
	// The terminal state is the segment.finished event in the stream (not a
	// separate channel) — the run tree (root + subagents) shares this
	// one stream (API.md §5 / §5.4).
	StartRun(ctx context.Context, in StartRunRequest) (*StartRunResponse, <-chan RunEvent, error)

	// ResumeRun answers open interrupts by opening a new segment of the SAME run
	// (R model, API.md §6.1): same stable runId, a fresh segmentId.
	ResumeRun(ctx context.Context, in ResumeRunRequest) (*StartRunResponse, <-chan RunEvent, error)

	// SubscribeRun rebinds a run's current live segment to the caller (reconnect /
	// crash recovery; subscribes the whole run tree).
	SubscribeRun(ctx context.Context, runID string) (*StartRunResponse, <-chan RunEvent, error)

	// CancelRun hard-stops a running run (outcome:canceled).
	CancelRun(ctx context.Context, in CancelRunRequest) error

	// SteerRun injects a user message into an actively-running run so the model
	// reads it on its next tool round (mid-run steering, API.md §6) — distinct
	// from runs.resume (which answers an interrupt) and runs.start (a new turn).
	// Errors run_not_found when the run isn't actively running (parked / done).
	SteerRun(ctx context.Context, in SteerRunRequest) error

	// ListRuns returns only running runs (API.md §7.3), as a Page.
	ListRuns(ctx context.Context, in ListRunsRequest) (*Page[RunRef], error)

	// ListOpenInterrupts returns durable resumable interrupts (API.md §6.2),
	// as a Page.
	ListOpenInterrupts(ctx context.Context, in ListOpenInterruptsRequest) (*Page[OpenInterrupt], error)
}

// RunStatus is the lifecycle status carried on RunRef (API.md §4.2).
type RunStatus string

const (
	RunStatusRunning  RunStatus = "running"
	RunStatusFinished RunStatus = "finished"
)

// RunRef identifies a run + its place in the run tree (API.md §4.2). ID is the
// STABLE logical run id — a resume continues the same run (a new segment), never
// a new run — so SpawnedByItemID (this run is a subagent of that toolCall item)
// is the only run-tree edge; there is no continuation chain to carry.
type RunRef struct {
	ID              string `json:"id"`
	SessionID       string `json:"sessionId"`
	SpawnedByItemID string `json:"spawnedByItemId,omitempty"`
	// Model is the model id this run ran against (Model.id); empty means the
	// run used the runtime default (surfaced via Session.model).
	Model string `json:"model,omitempty"`
	// Provider is the provider id this run ran against (Provider.id), paired
	// with Model. Empty means the runtime default. Stamped so a finished run is
	// self-describing — usage.summary attributes spend by provider without
	// re-deriving the model→provider mapping (which isn't 1:1 across compat
	// providers).
	Provider   string      `json:"provider,omitempty"`
	Status     RunStatus   `json:"status,omitempty"`
	Outcome    *RunOutcome `json:"outcome,omitempty"`
	CreatedAt  time.Time   `json:"createdAt,omitzero"`
	FinishedAt time.Time   `json:"finishedAt,omitzero"`
}

// RunOutcomeType discriminates the RunOutcome union (API.md §4.2).
type RunOutcomeType string

const (
	OutcomeCompleted RunOutcomeType = "completed"
	OutcomeError     RunOutcomeType = "error"
	OutcomeMaxSteps  RunOutcomeType = "maxSteps"
	OutcomeMaxBudget RunOutcomeType = "maxBudget"
	OutcomeCanceled  RunOutcomeType = "canceled"
	OutcomeInterrupt RunOutcomeType = "interrupt"
)

// RunOutcome is a tag-discriminated union over how a run ended (API.md §4.2).
//
//	completed/error/maxSteps/maxBudget/canceled → Result
//	interrupt                                   → Interrupts (resumable)
type RunOutcome struct {
	Type   RunOutcomeType `json:"type"`
	Result *RunResult     `json:"result,omitempty"`
	// Detail is a human-readable note for the non-error terminals
	// (maxSteps / maxBudget / canceled) — lets the client tell "user
	// canceled" from "timed out", show "$X / $Y" for maxBudget, etc. The
	// runs.cancel reason flows here (S6 / API.md §4.2). The error terminal's
	// note stays on Result.Error.Detail (§4.6), not duplicated here.
	Detail     string      `json:"detail,omitempty"`
	Interrupts []Interrupt `json:"interrupts,omitempty"`
}

// RunResult is a run's terminal metering (API.md §4.2). Total cost reads
// Usage.CostUSD — there is no separate RunResult.costUsd (it would be a
// second source of total cost; §4.2 / N1).
type RunResult struct {
	Usage *Usage       `json:"usage,omitempty"`
	Steps *int         `json:"steps,omitempty"`
	Error *ProblemData `json:"error,omitempty"` // present when outcome.type=error
	// DurationMs is the run's wall-clock duration in milliseconds (spans any
	// interrupt/resume cycles). Lets the client show a final "took 12.4s" on
	// any terminal — distinct from the live elapsed timer (which stops at the
	// terminal event). Omitted when zero / unmeasured.
	DurationMs int `json:"durationMs,omitempty"`
}

// StartRunRequest is the runs.start body (API.md §7.1). No client-supplied
// runId; no cwd (resolved from the session). (The X-Idempotency-Key retry
// dedup is reserved but not yet implemented — see API.md §7.1.)
type StartRunRequest struct {
	SessionID string         `json:"sessionId"`
	Input     []ContentBlock `json:"input"`
	Context   []ContextItem  `json:"context,omitempty"`
	Tools     []ToolSpec     `json:"tools,omitempty"`
	State     map[string]any `json:"state,omitempty"`
	// Provider + Model select the model for this run. They are paired: send
	// both to pick a model, or neither to use the runtime's default. Sending
	// one without the other is invalid_params — the provider is explicit,
	// never inferred from the model id. Both are meaningful slugs (no "Id"
	// suffix, mirroring `model` and Model.provider).
	Provider     string            `json:"provider,omitempty"`
	Model        string            `json:"model,omitempty"`
	MaxSteps     int               `json:"maxSteps,omitempty"`
	MaxBudgetUSD float64           `json:"maxBudgetUsd,omitempty"`
	Params       *GenerationParams `json:"params,omitempty"`
}

// StartRunResponse is the synchronous result of runs.start / resume /
// subscribe.
type StartRunResponse struct {
	RunID string `json:"runId"`
	// SegmentID is the streamed segment this call opened (a fresh one per
	// runs.start / runs.resume; the current live one for runs.subscribe). The
	// client keys its stream tree + reconnect-replay dedup on it (§0.3).
	SegmentID string `json:"segmentId"`
	// UserItemID is the id of the userMessage Item this run opens with — the
	// same id that rides the stream (item.started/completed) and lands in
	// items.list. Returned so a client can reconcile its optimistic user
	// bubble by exact id instead of matching on content. Empty for runs that
	// open no user turn (runs.resume). It's a business field, not transport
	// metadata.
	UserItemID string `json:"userItemId,omitempty"`
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

// SteerRunRequest is the runs.steer body — a user message to inject into the
// running run identified by RunID.
type SteerRunRequest struct {
	RunID   string `json:"runId"`
	Message string `json:"message"`
}

// ListRunsRequest is the runs.list body.
type ListRunsRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	PageQuery
}

// ListOpenInterruptsRequest is the runs.listOpenInterrupts body.
type ListOpenInterruptsRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	PageQuery
}

// ResumeRunRequest is the runs.resume body (API.md §6.1). RunID is the stable
// run to continue — its current segment parked with outcome:interrupt.
type ResumeRunRequest struct {
	RunID     string              `json:"runId"`
	Responses []InterruptResponse `json:"responses"`
}

// InterruptResponseType discriminates a client's answer to an interrupt
// (API.md §6.1). "answer" responds to a "question" interrupt.
type InterruptResponseType string

const (
	InterruptResponseApproval   InterruptResponseType = "approval"
	InterruptResponseAnswer     InterruptResponseType = "answer"
	InterruptResponseToolResult InterruptResponseType = "toolResult"
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
//	approval   → Decision, EditedArgs, Reason
//	answer     → Answers
//	toolResult → Result, Error
type InterruptResponse struct {
	ItemID   string                 `json:"itemId"`
	Response InterruptResponseValue `json:"response"`
}

// InterruptResponseValue is the discriminated response payload. toolResult
// carries the client-side tool's outcome the same shape as
// ToolInvocation.result (API.md §6.1): a best-effort JSON Result, or an
// Error when the client tool failed.
type InterruptResponseValue struct {
	Type       InterruptResponseType `json:"type"`                 // see InterruptResponseType
	Decision   ApprovalDecision      `json:"decision,omitempty"`   // approval: see ApprovalDecision
	Remember   *RememberScope        `json:"remember,omitempty"`   // approval: keep this decision (AUX_API §6)
	EditedArgs map[string]any        `json:"editedArgs,omitempty"` // approval: one-shot arg override
	Reason     string                `json:"reason,omitempty"`     // approval (deny rationale)
	Answers    map[string][]string   `json:"answers,omitempty"`    // answer: field name → values (single-select = one-element array, S8)
	Result     any                   `json:"result,omitempty"`     // toolResult: best-effort JSON
	Error      *ProblemData          `json:"error,omitempty"`      // toolResult: client tool failure
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
// approval, a question awaiting an answer, or a client-side tool to run.
type InterruptType string

const (
	InterruptApproval   InterruptType = "approval"
	InterruptQuestion   InterruptType = "question"
	InterruptToolResult InterruptType = "toolResult"
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
//	approval   -> Tool, optional Risk and Reason
//	question   -> Question
//	toolResult -> Tool
//
// The pointer fields retain the wire distinction between an absent member and
// a member whose value happens to be empty while avoiding an open-ended map at
// the protocol boundary.
type InterruptPayload struct {
	Tool     *ToolInvocation `json:"tool,omitempty"`
	Risk     ApprovalRisk    `json:"risk,omitempty"`
	Reason   string          `json:"reason,omitempty"`
	Question *Question       `json:"question,omitempty"`
}

// Interrupt is one pending HITL item (API.md §4.8). ItemID is the correlation
// key (the toolCall/question item awaiting resolution).
type Interrupt struct {
	ItemID  string            `json:"itemId"`
	Type    InterruptType     `json:"type"` // see InterruptType
	Payload *InterruptPayload `json:"payload,omitempty"`
}

// OpenInterrupt is a durable, resumable interrupt (API.md §4.8 / §6.2). RunID is
// the stable run to resume — its current segment parked with outcome:interrupt.
type OpenInterrupt struct {
	RunID      string      `json:"runId"`
	SessionID  string      `json:"sessionId"`
	Interrupts []Interrupt `json:"interrupts"`
	CreatedAt  time.Time   `json:"createdAt"`
}

// ContextItemType discriminates a ContextItem (API.md §4.7).
type ContextItemType string

const (
	ContextItemFile      ContextItemType = "file"
	ContextItemSelection ContextItemType = "selection"
	ContextItemURL       ContextItemType = "url"
)

// ContextItem is one piece of side-channel context attached to a run
// (API.md §4.7). Tag-discriminated by Type:
//
//	file      → Path (relative to Session.cwd)
//	selection → Path, Range ([startLine, endLine], 1-based inclusive)
//	url       → URL (runtime fetches; SSRF egress policy applies)
//
// Images aren't context items — they ride the run's input inline as an
// image ContentBlock (Mime + base64 Data), see StartRunRequest.Input.
//
// Security: file/selection paths relative to cwd; escaping cwd →
// path_outside_root. URL fetches block loopback / private / metadata.
type ContextItem struct {
	Type  ContextItemType `json:"type"`
	Path  string          `json:"path,omitempty"`
	Range []int           `json:"range,omitempty"`
	URL   string          `json:"url,omitempty"`
}

// ToolSpec is a client-supplied tool descriptor (API.md §4.7).
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`  // JSON Schema
	SafetyClass SafetyClass    `json:"safetyClass,omitempty"` // see SafetyClass
}
