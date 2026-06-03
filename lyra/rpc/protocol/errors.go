package protocol

import "errors"

// ProblemData is the structured error payload (API.md §4.6 / §8). It
// rides RPCError.data, RunResult.error, and toolCall.error. Type is the
// stable symbolic name — clients judge errors by Type, never by numeric
// code (API.md §8.2).
type ProblemData struct {
	Type      string `json:"type"`
	Detail    string `json:"detail,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
	// Extra carries any additional, error-specific fields. Marshaled
	// flat alongside the named fields by the dispatch layer.
	Extra map[string]any `json:"-"`
}

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
	CodeRunNotRunning          = -32007
	CodeRunAlreadyDone         = -32008
	CodeCheckpointUnavail      = -32009
	CodeAttachmentTooLarge     = -32010
	CodeUnsupportedMime        = -32011
	CodeToolDenied             = -32012
	CodePathOutsideRoot        = -32013
	CodeInterruptNotOpen       = -32014
	CodeIdempotencyConflict    = -32015
	CodeInvalidProtocolVersion = -32016
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
	ErrRunNotRunning          = errors.New("run_not_running")
	ErrRunAlreadyDone         = errors.New("run_already_finished")
	ErrCheckpointUnavailable  = errors.New("checkpoint_unavailable")
	ErrAttachmentTooLarge     = errors.New("attachment_too_large")
	ErrUnsupportedMime        = errors.New("unsupported_mime")
	ErrToolDenied             = errors.New("tool_denied")
	ErrPathOutsideRoot        = errors.New("path_outside_root")
	ErrInterruptNotOpen       = errors.New("interrupt_not_open")
	ErrIdempotencyConflict    = errors.New("idempotency_conflict")
	ErrInvalidProtocolVersion = errors.New("invalid_protocol_version")
)
