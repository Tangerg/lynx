package protocol

import "errors"

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
	// Retryable marks transient failures; RetryAfterSeconds, when given,
	// is the earliest sensible retry (e.g. a provider rate-limit backoff)
	// the client should honor before falling back to its own (API.md §8.3).
	Retryable         bool `json:"retryable,omitempty"`
	RetryAfterSeconds int  `json:"retryAfterSeconds,omitempty"`
	// Errors carries field-level validation failures (typically
	// invalid_params / form validation), addressable by field so the UI
	// can flag each one (API.md §8.3).
	Errors []FieldError `json:"errors,omitempty"`
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
	CodeToolDenied             = -32012
	CodePathOutsideRoot        = -32013
	CodeInterruptNotOpen       = -32014
	CodeInvalidProtocolVersion = -32016
	CodeVcsUnavailable         = -32017
	CodeSessionBusy            = -32018
)

// Sentinel errors returned by Runtime implementations. The dispatch
// maps each onto its {code, data.type} pair (API.md §8.2). Unrecognized
// errors map to internal_error.
var (
	ErrMethodNotFound         = errors.New("method_not_found")
	ErrInvalidParams          = errors.New("invalid_params")
	ErrProviderError          = errors.New("provider_error")
	ErrSessionNotFound        = errors.New("session_not_found")
	ErrRunNotFound            = errors.New("run_not_found")
	ErrItemNotFound           = errors.New("item_not_found")
	ErrCwdUnavailable         = errors.New("cwd_unavailable")
	ErrCapabilityNotNeg       = errors.New("capability_not_negotiated")
	ErrRunAlreadyDone         = errors.New("run_already_finished")
	ErrCheckpointUnavailable  = errors.New("checkpoint_unavailable")
	ErrUnsupportedMime        = errors.New("unsupported_mime")
	ErrToolDenied             = errors.New("tool_denied")
	ErrPathOutsideRoot        = errors.New("path_outside_root")
	ErrInterruptNotOpen       = errors.New("interrupt_not_open")
	ErrInvalidProtocolVersion = errors.New("invalid_protocol_version")
	// ErrVcsUnavailable: git is available but the cwd isn't a repo (AUX_API
	// §2.3) — distinct from "clean repo" (empty result). NOT for missing git
	// (that's features.git=false) nor an unresolvable base branch (invalid_params).
	ErrVcsUnavailable = errors.New("vcs_unavailable")
	// ErrSessionBusy: a session has a run in flight, so an operation that would
	// race the in-progress history append is refused (AUX_API §4.1 — rollback).
	ErrSessionBusy = errors.New("session_busy")
)
