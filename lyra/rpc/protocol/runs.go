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

	// ListRuns returns only running runs (API.md §7.3).
	ListRuns(ctx context.Context, in ListRunsRequest) ([]RunRef, error)

	// ListOpenInterrupts returns durable resumable interrupts (API.md §6.2).
	ListOpenInterrupts(ctx context.Context, in ListOpenInterruptsRequest) ([]OpenInterrupt, error)
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
	Type       RunOutcomeType `json:"type"`
	Result     *RunResult     `json:"result,omitempty"`
	Interrupts []Interrupt    `json:"interrupts,omitempty"`
}

// RunResult is a run's terminal metering (API.md §4.2).
type RunResult struct {
	Usage   *Usage       `json:"usage,omitempty"`
	CostUSD *float64     `json:"costUsd,omitempty"` // omitted when no pricing (not faked to 0)
	Steps   *int         `json:"steps,omitempty"`
	Error   *ProblemData `json:"error,omitempty"` // present when outcome.type=error
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
	SessionID    string            `json:"sessionId"`
	Input        []ContentBlock    `json:"input"`
	Context      []ContextItem     `json:"context,omitempty"`
	Tools        []ToolSpec        `json:"tools,omitempty"`
	State        map[string]any    `json:"state,omitempty"`
	Attachments  []string          `json:"attachments,omitempty"`
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
}

// ListOpenInterruptsRequest is the runs.listOpenInterrupts body.
type ListOpenInterruptsRequest struct {
	SessionID string `json:"sessionId,omitempty"`
}

// ResumeRunRequest is the runs.resume body (API.md §6.1).
type ResumeRunRequest struct {
	ParentRunID string              `json:"parentRunId"`
	Responses   []InterruptResponse `json:"responses"`
}

// InterruptResponse answers one open interrupt, keyed by itemId (API.md §6.1).
// Response is a tag-discriminated union (Kind):
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
	Kind       string         `json:"kind"`                 // "approval" | "answer" | "toolResult"
	Decision   string         `json:"decision,omitempty"`   // approval: "approve" | "deny"
	EditedArgs map[string]any `json:"editedArgs,omitempty"` // approval
	Reason     string         `json:"reason,omitempty"`     // approval (deny rationale)
	Answers    map[string]any `json:"answers,omitempty"`    // answer: field name → label(s) / free text
	Result     any            `json:"result,omitempty"`     // toolResult: best-effort JSON
	Error      *ProblemData   `json:"error,omitempty"`      // toolResult: client tool failure
}

// Interrupt is one pending HITL item (API.md §4.8). itemId is the
// correlation key (the toolCall/question item awaiting resolution).
type Interrupt struct {
	ItemID  string         `json:"itemId"`
	Kind    string         `json:"kind"` // "approval" | "question" | "toolResult"
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
// (API.md §4.7). Tag-discriminated by Kind:
//
//	file      → Path (relative to Session.cwd)
//	selection → Path, Range ([startLine, endLine], 1-based inclusive)
//	url       → URL (runtime fetches; SSRF egress policy applies)
//	image     → AttachmentID
//
// Security: file/selection paths relative to cwd; escaping cwd →
// path_outside_root. URL fetches block loopback / private / metadata.
type ContextItem struct {
	Kind         string `json:"kind"`
	Path         string `json:"path,omitempty"`
	Range        []int  `json:"range,omitempty"`
	URL          string `json:"url,omitempty"`
	AttachmentID string `json:"attachmentId,omitempty"`
}

// ToolSpec is a client-supplied tool descriptor (API.md §4.7).
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema
	SafetyClass string         `json:"safetyClass,omitempty"`
}
