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
	// The terminal state is the run.finished event in the stream (not a
	// separate channel) — the run tree (root + subagents) shares this
	// one stream (API.md §5 / §5.4).
	StartRun(ctx context.Context, in StartRunRequest) (*StartRunResponse, <-chan RunEvent, error)

	// ResumeRun answers open interrupts by starting a continuation run
	// (R model, API.md §6.1). The new run's RunRef.parentRunId = the
	// interrupted run id.
	ResumeRun(ctx context.Context, in ResumeRunRequest) (*StartRunResponse, <-chan RunEvent, error)

	// SubscribeRun rebinds an existing root run's stream to the caller
	// (reconnect / crash recovery; subscribes the whole run tree).
	SubscribeRun(ctx context.Context, runID string) (*StartRunResponse, <-chan RunEvent, error)

	// CancelRun hard-stops a running run (outcome:canceled).
	CancelRun(ctx context.Context, in CancelRunRequest) error

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

// RunRef identifies a run + its place in the run tree (API.md §4.2).
//
//	SpawnedByItemID (child-of)  → this run is a subagent of that toolCall item
//	ParentRunID    (continuation-of) → this run continues that run (resume/edit)
//
// The two are never reused for each other's meaning.
type RunRef struct {
	ID              string      `json:"id"`
	SessionID       string      `json:"sessionId"`
	SpawnedByItemID string      `json:"spawnedByItemId,omitempty"`
	ParentRunID     string      `json:"parentRunId,omitempty"`
	Status          RunStatus   `json:"status,omitempty"`
	Outcome         *RunOutcome `json:"outcome,omitempty"`
	CreatedAt       time.Time   `json:"createdAt,omitzero"`
	FinishedAt      time.Time   `json:"finishedAt,omitzero"`
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
}

// RunMode is the optional execution mode hint (API.md §7.1).
type RunMode string

const (
	RunModeAgent RunMode = "agent"
	RunModeChat  RunMode = "chat"
	RunModePlan  RunMode = "plan"
)

// StartRunRequest is the runs.start body (API.md §7.1). No client-supplied
// runId (idempotency is the X-Idempotency-Key header); no cwd (resolved
// from the session).
type StartRunRequest struct {
	SessionID   string         `json:"sessionId"`
	Input       []ContentBlock `json:"input"`
	Context     []ContextItem  `json:"context,omitempty"`
	Tools       []ToolSpec     `json:"tools,omitempty"`
	State       map[string]any `json:"state,omitempty"`
	// Provider + Model select the model for this run. They are paired: send
	// both to pick a model, or neither to use the runtime's default. Sending
	// one without the other is invalid_params — the provider is explicit,
	// never inferred from the model id. Both are meaningful slugs (no "Id"
	// suffix, mirroring `model` and Model.provider).
	Provider     string            `json:"provider,omitempty"`
	Model        string            `json:"model,omitempty"`
	Mode         RunMode           `json:"mode,omitempty"`
	MaxSteps     int               `json:"maxSteps,omitempty"`
	MaxBudgetUSD float64           `json:"maxBudgetUsd,omitempty"`
	Params       *GenerationParams `json:"params,omitempty"`
}

// StartRunResponse is the synchronous result of runs.start / resume /
// subscribe.
type StartRunResponse struct {
	RunID string `json:"runId"`
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

// ResumeRunRequest is the runs.resume body (API.md §6.1).
type ResumeRunRequest struct {
	ParentRunID string              `json:"parentRunId"`
	Responses   []InterruptResponse `json:"responses"`
}

// InterruptResponseType discriminates a client's answer to an interrupt
// (API.md §6.1). "answer" responds to a "question" interrupt.
type InterruptResponseType string

const (
	InterruptResponseApproval   InterruptResponseType = "approval"
	InterruptResponseAnswer     InterruptResponseType = "answer"
	InterruptResponseToolResult InterruptResponseType = "toolResult"
)

// Valid reports whether t is a known interrupt-response type.
func (t InterruptResponseType) Valid() bool {
	return t == InterruptResponseApproval || t == InterruptResponseAnswer || t == InterruptResponseToolResult
}

// ApprovalDecision is the verdict on an approval interrupt (API.md §6.1).
type ApprovalDecision string

const (
	ApprovalApprove ApprovalDecision = "approve"
	ApprovalDeny    ApprovalDecision = "deny"
)

// Valid reports whether d is a known decision.
func (d ApprovalDecision) Valid() bool { return d == ApprovalApprove || d == ApprovalDeny }

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
// §6). v1 honors "session" only; "project"/"global" are accepted but degrade to
// one-shot (no persistence home yet).
type RememberScopeKind string

const (
	RememberSession RememberScopeKind = "session"
	RememberProject RememberScopeKind = "project"
	RememberGlobal  RememberScopeKind = "global"
)

// RememberScope is the standing-decision directive on an approval response
// (AUX_API §6). When present the runtime keeps the approve/deny decision so
// future calls to the same tool (keyed by tool NAME, not its args) skip the
// prompt. v1 honors Scope "session" only — in-memory, process lifetime;
// "project" / "global" need a persistence home and aren't wired, so a client
// sending them gets one-shot behavior rather than a false promise. editedArgs
// stays one-shot regardless: remember records "this tool", not "this tool +
// these args".
type RememberScope struct {
	Scope RememberScopeKind `json:"scope"` // see RememberScopeKind (v1 honors session)
}

// Interrupt is one pending HITL item (API.md §4.8). itemId is the
// correlation key (the toolCall/question item awaiting resolution).
// InterruptType discriminates a pending interrupt (API.md §4.8): a tool awaiting
// approval, a question awaiting an answer, or a client-side tool to run.
type InterruptType string

const (
	InterruptApproval   InterruptType = "approval"
	InterruptQuestion   InterruptType = "question"
	InterruptToolResult InterruptType = "toolResult"
)

type Interrupt struct {
	ItemID  string         `json:"itemId"`
	Type    InterruptType  `json:"type"` // see InterruptType
	Payload map[string]any `json:"payload,omitempty"`
}

// OpenInterrupt is a durable, resumable interrupt (API.md §4.8 / §6.2).
type OpenInterrupt struct {
	ParentRunID string      `json:"parentRunId"`
	SessionID   string      `json:"sessionId"`
	Interrupts  []Interrupt `json:"interrupts"`
	CreatedAt   time.Time   `json:"createdAt"`
}

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
// ContextItemType discriminates a ContextItem (API.md §4.7).
type ContextItemType string

const (
	ContextItemFile      ContextItemType = "file"
	ContextItemSelection ContextItemType = "selection"
	ContextItemURL       ContextItemType = "url"
)

// Valid reports whether t is a known context-item type.
func (t ContextItemType) Valid() bool {
	return t == ContextItemFile || t == ContextItemSelection || t == ContextItemURL
}

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
