package protocol

import (
	"errors"
	"fmt"
)

// ErrorChannel self-describes which delivery channel an error arrived on
// (API.md §8.1): a sync JSON-RPC error, a run's error outcome, or a tool
// failure. Empty = unclassified.
type ErrorChannel string

const (
	ErrorChannelRPC  ErrorChannel = "rpc"
	ErrorChannelRun  ErrorChannel = "run"
	ErrorChannelTool ErrorChannel = "tool"
)

// ProblemData is the structured error payload (API.md §4.6 / §8) — a
// transport-agnostic trim of RFC 9457 Problem Details. It rides
// RPCError.data, RunResult.error, and toolCall.error. Type is the stable
// symbolic name — clients judge errors by Type, never by numeric code
// (API.md §8.2). First-party types are bare snake_case; third-party
// plugins namespace as `plugin:<name>/<symbol>` — one instance of the
// unified extension-namespace convention (API.md §2.5, error case §8.4).
type ProblemData struct {
	Type string `json:"type"`
	// Channel self-describes which delivery channel the error came on —
	// "rpc" (sync JSON-RPC error), "run" (segment.finished outcome:error), or
	// "tool" (toolCall.error) — so the client reads it instead of inferring
	// from where the error arrived (API.md §8.1). Empty = unclassified.
	Channel ErrorChannel `json:"channel,omitempty"`
	Detail  string       `json:"detail,omitempty"` // per-occurrence human-readable note
	// DocURL optionally points at this type's docs (Stripe doc_url), lowering
	// integration cost (API.md §8.3); absent → look the symbolic type up in §8.2.
	DocURL string `json:"docUrl,omitempty"`
	// Retryable has no writer: it left the domain with the transient/permanent
	// classification the contract rejects, and it leaves the wire with the
	// protocol bump. Do NOT restore it by deriving it here from Type — that is
	// presentation deciding a business fact, which is what the domain just
	// stopped doing. Clients branch on Type.
	Retryable bool `json:"retryable,omitempty"`
	// RetryAfterSeconds, when given, is the earliest sensible retry (e.g. a
	// provider rate-limit backoff) the client honors before its own (API.md §8.3).
	// Only the kinds that waiting can clear carry one.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`
	// ActiveRun is required by session_has_active_run and appears on no other type:
	// it names the run that made the request impossible, so the client can offer
	// steer / resume / cancel instead of just reporting a failure.
	ActiveRun *ActiveRunRef `json:"activeRun,omitempty"`
	// Errors carries field-level validation failures (typically
	// invalid_params / form validation), addressable by field so the UI
	// can flag each one (API.md §8.3).
	Errors []FieldError `json:"errors,omitempty"`
}

// ActiveRunRef is the run a session already holds, carried by
// session_has_active_run (§8.2). It is a snapshot taken at the admission boundary,
// not a lasting assertion: the client picks its next move from the status — steer a
// running run, resume or cancel a waiting one — and refreshes with runs.get before
// acting on it.
type ActiveRunRef struct {
	RunID  string    `json:"runId"`
	Status RunStatus `json:"status"`
}

// ActiveRunConflict is the session_has_active_run problem with its required
// payload. It is an error rather than a return value because it IS the refusal:
// nothing was created, so there is no result to carry it on.
type ActiveRunConflict struct {
	ActiveRun ActiveRunRef
}

func (e *ActiveRunConflict) Error() string {
	return fmt.Sprintf("%s: run %s is %s", ErrSessionHasActiveRun, e.ActiveRun.RunID, e.ActiveRun.Status)
}

// Is makes the typed conflict answer to its sentinel, so every reader that
// branches on problem type keeps working and only a reader that needs the payload
// asks for the type.
func (e *ActiveRunConflict) Is(target error) bool { return target == ErrSessionHasActiveRun }

// Enrich fills the structured fields this problem type requires (§8.2's frame
// table). It exists so the field travels with the error that knows it, instead of
// delivery re-deriving it where the error is turned into a frame.
func (e *ActiveRunConflict) Enrich(data *ProblemData) { data.ActiveRun = &e.ActiveRun }

// ProblemDetailed is implemented by an error whose problem type requires structured
// fields beyond the prose detail. The dispatcher applies it when building the frame.
type ProblemDetailed interface {
	error
	Enrich(*ProblemData)
}

// FieldError is one field-level validation failure inside ProblemData
// (API.md §4.6 / §8.3). Field is the offending params key.
type FieldError struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

// First-party ProblemData.Type symbols for the run and tool channels (API.md
// §8.2). ProblemData.Type stays an open string — the RPC-channel symbols (the
// Err* sentinels below) and plugin-namespaced `plugin:<name>/<symbol>` types
// also ride it — so these are named constants for the first-party set, not a
// closed enum: the wire value is the string itself; production assigns the
// constant (no typo drift), tests assert the literal (pins the wire value).
const (
	// ProblemInternalError is the unclassified-failure fallback on every channel
	// (run outcome:error, RPC error, tool error); the full error rides the span,
	// never the wire.
	ProblemInternalError = "internal_error"
	// Run channel (segment.finished outcome:error) — how a failed run is classified.
	ProblemRunLost             = "run_lost"             // process exited before the run reached a durable terminal
	ProblemAgentStuck          = "agent_stuck"          // the loop's no-forward-progress guard tripped
	ProblemRateLimited         = "rate_limited"         // provider 429 / quota — retryable
	ProblemInvalidAPIKey       = "invalid_api_key"      // provider 401 / 403 — not retryable
	ProblemTimeout             = "timeout"              // provider request timed out / connection failed — retryable
	ProblemProviderUnavailable = "provider_unavailable" // provider 5xx — retryable
	ProblemProviderRejected    = "provider_rejected"    // provider 400, request rejected as invalid — not retryable
	// Tool channel (toolCall.error) — how a tool call failed.
	ProblemDeniedByUser = "denied_by_user" // denied by the approval verdict
	ProblemToolFailed   = "tool_failed"    // tool execution returned an error
	// Inline status (McpServer.error, ProviderTestResult.error) — a connection or
	// probe verdict that rides its own query result instead of failing the call,
	// so the pane renders it beside the thing it describes.
	ProblemMCPAuthorizationRequired = "mcp_authorization_required" // an HTTP MCP server needs an interactive sign-in
	ProblemMCPDialFailed            = "mcp_dial_failed"            // the MCP connection, or a test of it, did not succeed
	ProblemProviderNotConfigured    = "provider_not_configured"    // the provider has no credential yet
	ProblemProviderTestFailed       = "provider_test_failed"       // the provider was unreachable or rejected the probe
)

// Error code <-> symbolic name table (API.md §8.2). Numeric codes are
// v2-fresh; the dispatch maps these sentinels onto {code, data.type}.
const (
	CodeInvalidRequest         = -32600
	CodeMethodNotFound         = -32601
	CodeInvalidParams          = -32602
	CodeInternalError          = -32603
	CodeProviderError          = -32001
	CodeSessionNotFound        = -32002
	CodeRunNotFound            = -32003
	CodeItemNotFound           = -32004
	CodeCwdUnavailable         = -32005
	CodeCapabilityNotNeg       = -32006
	CodeRunAlreadyDone         = -32008
	CodeCheckpointUnavail      = -32009
	CodeUnsupportedMime        = -32011
	CodePathOutsideRoot        = -32013
	CodeInterruptNotOpen       = -32014
	CodeInvalidProtocolVersion = -32016
	CodeVcsUnavailable         = -32017
	CodeSessionBusy            = -32018
	CodeRevisionConflict       = -32019
	CodeIdempotencyConflict    = -32020
	CodeIdempotencyInProgress  = -32021
	// -32007 / -32010 / -32012 / -32015 are retired holes, never reused, so a new
	// code continues the sequence rather than filling one in.
	CodeRunNotRoot          = -32022
	CodeSessionHasActiveRun = -32023
)

// Sentinel errors returned by Runtime implementations. The dispatch
// maps each onto its {code, data.type} pair (API.md §8.2). Unrecognized
// errors map to internal_error.
var (
	ErrMethodNotFound        = errors.New("method_not_found")
	ErrInvalidParams         = errors.New("invalid_params")
	ErrProviderError         = errors.New("provider_error")
	ErrSessionNotFound       = errors.New("session_not_found")
	ErrRunNotFound           = errors.New("run_not_found")
	ErrItemNotFound          = errors.New("item_not_found")
	ErrCwdUnavailable        = errors.New("cwd_unavailable")
	ErrCapabilityNotNeg      = errors.New("capability_not_negotiated")
	ErrRunAlreadyDone        = errors.New("run_already_finished")
	ErrCheckpointUnavailable = errors.New("checkpoint_unavailable")
	ErrUnsupportedMime       = errors.New("unsupported_mime")
	ErrPathOutsideRoot       = errors.New("path_outside_root")
	ErrInterruptNotOpen      = errors.New("interrupt_not_open")
	// ErrSessionHasActiveRun: runs.start found the session already holding a
	// non-terminal root run. The runtime does NOT cancel it — which run continues is
	// a decision only the person can make, and an implicit cancel would throw work
	// away to serve a request that could have been a steer. See [ActiveRunConflict],
	// which carries the run so the client can offer the choice.
	ErrSessionHasActiveRun = errors.New("session_has_active_run")
	// ErrRunNotRoot: a root-only operation named a child run. It is not
	// run_not_found — the run exists, and the thing the caller wants exists under its
	// root — so the remedy is to follow rootRunId, not to look for a different id.
	ErrRunNotRoot             = errors.New("run_not_root")
	ErrInvalidProtocolVersion = errors.New("invalid_protocol_version")
	// ErrVcsUnavailable: git is available but the cwd isn't a repo (AUX_API
	// §2.3) — distinct from "clean repo" (empty result). NOT for missing git
	// (that's features.git=false) nor an unresolvable base branch (invalid_params).
	ErrVcsUnavailable = errors.New("vcs_unavailable")
	// ErrSessionBusy: a session has a run in flight, so an operation that would
	// race the in-progress history append is refused (AUX_API §4.1 — rollback).
	ErrSessionBusy = errors.New("session_busy")
	// ErrRevisionConflict: a conditional mutation used a stale resource revision.
	ErrRevisionConflict      = errors.New("revision_conflict")
	ErrIdempotencyConflict   = errors.New("idempotency_conflict")
	ErrIdempotencyInProgress = errors.New("idempotency_in_progress")
)
