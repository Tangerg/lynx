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
	RunID   string          `json:"runId"`
	EventID string          `json:"eventId"`
	Event   json.RawMessage `json:"event"`
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
	StartRun(ctx context.Context, in StartRunRequest) (*StartRunResponse, <-chan AgUiEvent, error)

	// CancelRun stops an in-flight run. Backs the runs.cancel JSON-RPC
	// Request (API.md v4 §3.5): a proper Request method that returns
	// void, NOT a notification. notifications/cancelled is reserved for
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
