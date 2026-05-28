// Package transport is the Lyra Runtime Protocol's transport layer
// — a bidirectional pipe for JSON-RPC 2.0 messages. See
// docs/TRANSPORT.md for the architectural picture.
//
// One [Transport] interface, three implementations in scope:
//
//   - pkg/transport/inprocess — Go ↔ Go in the same binary, business
//     path bypasses JSON serialisation entirely.
//   - pkg/transport/http      — JSON-RPC over HTTP (POST /v1/rpc[/{method}])
//     + SSE notifications (GET /v1/rpc/stream) + sidecar /v1/info,
//     /v1/health.
//   - pkg/transport/wails     — Wails IPC (WebView ↔ host) — deferred.
//
// Pairing of requests with responses (matching by Message.ID) is the
// RPC client's job, not the transport's. Streaming is just an
// uninterrupted run of [Notification]s on the inbound side — no
// special framing.
package transport

import (
	"context"
	"encoding/json"
)

// JSONRPCVersion is the protocol version every message MUST carry
// (JSON-RPC 2.0 spec).
const JSONRPCVersion = "2.0"

// Message is one JSON-RPC 2.0 envelope. Per spec, only the relevant
// fields are populated:
//
//   - Request:      jsonrpc + id + method + params?
//   - Response:     jsonrpc + id + (result XOR error)
//   - Notification: jsonrpc + method + params?    (no id)
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the JSON-RPC error envelope (API.md §7).
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error makes RPCError satisfy the error interface so it can flow
// through normal Go error returns.
func (e *RPCError) Error() string {
	if e == nil {
		return "<nil rpc error>"
	}
	return e.Message
}

// IsRequest reports whether the message carries an id (Request or
// Response) — Notifications have no id field.
func (m *Message) IsRequest() bool { return m.ID != nil && m.Method != "" }

// IsResponse reports whether this looks like a Response — has id,
// no method.
func (m *Message) IsResponse() bool { return m.ID != nil && m.Method == "" }

// IsNotification reports whether this is a Notification — has
// method, no id.
func (m *Message) IsNotification() bool { return m.ID == nil && m.Method != "" }

// Transport is the bidirectional message pipe. One interface,
// multiple implementations.
//
// Concurrency: implementations MUST be safe for concurrent Send
// from multiple goroutines, and Recv MUST yield to exactly one
// consumer. Close MUST be idempotent.
type Transport interface {
	// Send hands one outbound message to the underlying transport.
	// Returns when the message has been queued, not when the peer
	// has processed it. Honors ctx for cancellation / timeout.
	Send(ctx context.Context, msg *Message) error

	// Recv returns the inbound channel. The channel closes when the
	// transport disconnects; consumers MUST drain it (or risk
	// goroutine leaks on the sender side).
	Recv() <-chan *Message

	// Close terminates the transport. Pending Sends fail with
	// context.Canceled; Recv channel closes.
	Close() error
}

// JSON-RPC error codes — the standard band (-32700..-32603) plus
// the Lyra business band (-32000..-32099). The codes here are stable
// wire identifiers; the messages are advisory.
const (
	// Standard JSON-RPC errors.
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// Lyra business errors (API.md §7.2).
	CodeProviderError           = -32001
	CodeProviderRateLimited     = -32002
	CodeToolFailed              = -32003
	CodeApprovalRequired        = -32004
	CodeSessionNotFound         = -32005
	CodeMessageNotFound         = -32006
	CodeRunNotFound             = -32007
	CodeAttachmentTooLarge      = -32008
	CodeCapabilityNotNegotiated = -32009
	CodeInvalidProtocolVersion  = -32010
	CodeProtocolViolation       = -32011
)

// CodeMessage is the canonical error-message string for a given code
// — used by error envelopes when the impl didn't provide a more
// specific one.
func CodeMessage(code int) string {
	switch code {
	case CodeParseError:
		return "parse error"
	case CodeInvalidRequest:
		return "invalid request"
	case CodeMethodNotFound:
		return "method not found"
	case CodeInvalidParams:
		return "invalid params"
	case CodeInternalError:
		return "internal error"
	case CodeProviderError:
		return "provider_error"
	case CodeProviderRateLimited:
		return "provider_rate_limited"
	case CodeToolFailed:
		return "tool_failed"
	case CodeApprovalRequired:
		return "approval_required"
	case CodeSessionNotFound:
		return "session_not_found"
	case CodeMessageNotFound:
		return "message_not_found"
	case CodeRunNotFound:
		return "run_not_found"
	case CodeAttachmentTooLarge:
		return "attachment_too_large"
	case CodeCapabilityNotNegotiated:
		return "capability_not_negotiated"
	case CodeInvalidProtocolVersion:
		return "invalid_protocol_version"
	case CodeProtocolViolation:
		return "protocol_violation"
	default:
		return "unknown_error"
	}
}

// NewError builds an RPCError with the canonical message for the
// code and an optional data payload (typically the ProblemData
// shape from API.md §7.2).
func NewError(code int, data json.RawMessage) *RPCError {
	return &RPCError{Code: code, Message: CodeMessage(code), Data: data}
}

// NewErrorWithMessage is the same as NewError but lets the caller
// override the message — useful when wrapping a downstream Go
// error's String() to surface its detail.
func NewErrorWithMessage(code int, msg string, data json.RawMessage) *RPCError {
	return &RPCError{Code: code, Message: msg, Data: data}
}

// ProblemData mirrors RFC 7807 ProblemDetails, used as the Data
// payload on RPCError (API.md §7.2).
type ProblemData struct {
	Type         string            `json:"type,omitempty"`
	Detail       string            `json:"detail,omitempty"`
	RetryAfterMs int               `json:"retryAfterMs,omitempty"`
	Errors       []FieldError      `json:"errors,omitempty"`
}

// FieldError is one entry in ProblemData.Errors — used to point at a
// specific field in invalid params.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
