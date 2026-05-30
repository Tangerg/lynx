package protocol

import (
	"context"
	"encoding/json"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// AgUiEvent is the AG-UI event type re-exported under protocol —
// keeps downstream importers scoped to one package. Implementations
// MUST be able to marshal to JSON via ToJSON (the AG-UI SDK contract).
type AgUiEvent = aguievents.Event

// RunEvent is one event delivered on a run's stream. The
// notifications/run/event JSON-RPC notification's params is exactly
// this shape (API.md §3.1).
//
// API.md v4 greenfield cut: the older `streamHandle` field was a
// pure alias of RunID — collapsed into a single RunID so the
// notification carries the same resource id the StartRun result
// returned.
type RunEvent struct {
	RunID   string `json:"runId"`
	EventID string `json:"eventId"`
	// Ts is the server-authoritative timestamp (ISO-8601), present on
	// every event (API.md §3.1).
	Ts string `json:"ts"`
	// ParentToolUseID attributes the event to a sub-agent derived from a
	// tool-use; empty = main agent's event. Reserved until sub-agents
	// surface their own event source (API.md §3.1).
	ParentToolUseID string          `json:"parentToolUseId,omitempty"`
	Event           json.RawMessage `json:"event"`
}

// RunMode is the optional execution mode hint (API.md §6.3).
type RunMode string

const (
	RunModeAgent RunMode = "agent"
	RunModeChat  RunMode = "chat"
	RunModePlan  RunMode = "plan"
)

// Runs is the runs.* method group.
type Runs interface {
	// StartRun returns synchronously with the runId the client uses to
	// match incoming notifications/run/event entries; the actual
	// stream flows out through the transport's notification path
	// (Recv() channel for InProcess / SSE for HTTP).
	//
	// The returned event channel is for in-process consumers (TUI /
	// tests / server wiring). HTTP/Wails adapters pipe it into
	// the transport's outbound notification stream and don't expose
	// it to the wire client directly.
	// StartRun returns the runId, the AG-UI event stream, and a
	// single-shot terminal channel that yields one RunResult when the
	// run ends (then closes). Transports drain events, then read the
	// RunResult to build notifications/run/closed (API.md §3.1 / §6.3) —
	// terminal state + metering are read here, not by parsing the last
	// event.
	StartRun(ctx context.Context, in StartRunRequest) (*StartRunResponse, <-chan AgUiEvent, <-chan RunResult, error)

	// CancelRun stops an in-flight run. Backs the runs.cancel JSON-RPC
	// Request (API.md v4 §3.5): a proper Request method that returns
	// void, NOT a notification. notifications/canceled is reserved for
	// aborting an in-flight JSON-RPC Request (different semantic, see
	// API.md §2.4).
	CancelRun(ctx context.Context, runID string) error

	// SubmitApproval is runs.approval.submit — the client-side HITL
	// decision. The server validates the requestId against pending
	// approvals and resumes the matching run.
	SubmitApproval(ctx context.Context, in ApprovalRequest) error
}

// StartRunRequest is the runs.start request payload (API.md §6.3).
type StartRunRequest struct {
	SessionID   string         `json:"sessionId"`
	RunID       string         `json:"runId,omitempty"`
	Messages    []Message      `json:"messages"`
	State       map[string]any `json:"state,omitempty"`
	Tools       []ToolSpec     `json:"tools,omitempty"`
	Context     []ContextItem  `json:"context,omitempty"`
	Model       string         `json:"model,omitempty"`
	Mode        RunMode        `json:"mode,omitempty"`
	Attachments []string       `json:"attachments,omitempty"`

	// MaxTurns caps the tool-loop rounds; hitting it ends the run with
	// RunResult.stopReason="max_turns". Engine consumption TBD (chat has
	// no turn-count cap yet) — declared so the wire shape matches §6.3.
	MaxTurns int `json:"maxTurns,omitempty"`
	// MaxBudgetUSD caps cumulative cost (subtree-inclusive); overrun ends
	// with stopReason="max_budget". Wired to chat.StartTurnRequest.MaxCostUSD.
	MaxBudgetUSD float64 `json:"maxBudgetUsd,omitempty"`
	// Params is optional generation tuning (§6.3). Engine consumption TBD.
	Params *GenerationParams `json:"params,omitempty"`
}

// GenerationParams is optional LLM generation tuning (API.md §6.3),
// aligned with chat.Options. Pointers distinguish "unset" from a real
// zero so the engine only overrides what the client explicitly sent.
type GenerationParams struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxTokens       *int64   `json:"maxTokens,omitempty"`
	MaxOutputTokens *int64   `json:"maxOutputTokens,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	Stop            []string `json:"stop,omitempty"`
}

// StartRunResponse is the runs.start result. RunID is server-assigned
// when StartRunRequest.RunID is empty.
//
// API.md v4 greenfield cut: no `streamHandle` field — the runId IS
// the stream identifier. notifications/run/event carries the same
// runId in its params for client-side filtering.
type StartRunResponse struct {
	RunID string `json:"runId"`
}

// CancelRunRequest is the runs.cancel request payload (API.md v4 §3.5).
type CancelRunRequest struct {
	RunID  string `json:"runId"`
	Reason string `json:"reason,omitempty"`
}

// ApprovalRequest is the runs.approval.submit payload (§4.3). Decision is
// the two-value wire enum "approve" | "deny" (API.md v4 §4.2).
type ApprovalRequest struct {
	RequestID string `json:"requestId"`
	Decision  string `json:"decision"` // "approve" | "deny"
	Reason    string `json:"reason,omitempty"`
}

// JsonSchema is the wire type for any field that carries a JSON Schema
// (draft 2020-12) object — currently just ToolSpec.Parameters. Aliased
// to json.RawMessage at the Go level so the dispatcher doesn't
// fully decode the schema; the alias clarifies intent for codegen +
// downstream typed clients (API.md v4 §6.2).
type JsonSchema = json.RawMessage

// ToolSpec is a client-supplied tool descriptor (rare — most tools
// are server-side). origin tells the UI where the tool runs.
type ToolSpec struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Parameters  JsonSchema `json:"parameters"`
	Origin      string     `json:"origin"` // "server" | "client" | "mcp"
}

// ContextItem is one piece of side-channel context attached to a run
// (a file path, URL, code selection, image attachment).
//
// API.md v4 §6.2: discriminated union with flat per-kind fields.
// Each ContextItem carries Kind plus exactly the fields its kind
// declares. New kinds extend the union; do NOT introduce a generic
// {kind, data: map[string]any} bag.
//
// Kind = "file"       → Path
// Kind = "url"        → URL
// Kind = "selection"  → Path + Range (2-int [start, end])
// Kind = "image"      → AttachmentID
type ContextItem struct {
	Kind         string `json:"kind"`
	Path         string `json:"path,omitempty"`
	URL          string `json:"url,omitempty"`
	Range        []int  `json:"range,omitempty"`
	AttachmentID string `json:"attachmentId,omitempty"`
}

// RunResult is a run's terminal state, delivered once in
// notifications/run/closed (API.md §3.1 / §6.3). Stop reason + metering
// are read from here, not by parsing the last AG-UI event.
type RunResult struct {
	// StopReason: "completed" | "canceled" | "error" | "max_turns" | "max_budget".
	StopReason string    `json:"stopReason"`
	Usage      *Usage    `json:"usage,omitempty"`
	CostUSD    *float64  `json:"costUsd,omitempty"` // omitted when no pricing hook (not faked to 0)
	Turns      *int      `json:"turns,omitempty"`   // tool-loop rounds; omitted until the engine surfaces it
	Error      *RunError `json:"error,omitempty"`   // present when StopReason == "error"
}

// RunError is the structured error inside RunResult — same {code,message}
// shape as the §7 JSON-RPC error (code is a §7.2 business code).
type RunError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Usage is the cumulative token usage for a run (subtree-inclusive,
// API.md §6.3).
type Usage struct {
	InputTokens      int64                 `json:"inputTokens"`
	OutputTokens     int64                 `json:"outputTokens"`
	ReasoningTokens  int64                 `json:"reasoningTokens,omitempty"`
	CacheReadTokens  int64                 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64                 `json:"cacheWriteTokens,omitempty"`
	ByModel          map[string]ModelUsage `json:"byModel,omitempty"`
}

// ModelUsage is one model's slice of a run's tokens + cost.
type ModelUsage struct {
	InputTokens  int64    `json:"inputTokens"`
	OutputTokens int64    `json:"outputTokens"`
	CostUSD      *float64 `json:"costUsd,omitempty"`
}
