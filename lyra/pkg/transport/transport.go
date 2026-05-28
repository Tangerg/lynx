// Package transport is the Lyra Runtime Protocol's transport layer
// — a bidirectional pipe for JSON-RPC 2.0 messages. See
// docs/TRANSPORT.md for the architectural picture.
//
// One [Transport] interface, three implementations in scope:
//
//   - pkg/transport/inprocess — Go ↔ Go in the same binary, business
//     path bypasses JSON serialisation entirely.
//   - pkg/transport/http      — JSON-RPC over HTTP (POST /v1/rpc/{method})
//     + SSE notifications (GET /v1/rpc/stream) + sidecar /v1/info,
//     /v1/health.
//   - pkg/transport/wails     — Wails IPC (WebView ↔ host) — deferred.
//
// Wire envelope types and encode/decode are re-exported from the MCP
// Go SDK's `jsonrpc` package — same vendor we use for our MCP
// integration, conformant JSON-RPC 2.0 implementation, "for use by
// mcp transport authors" per its own doc.
//
// Lyra extensions on top of the SDK:
//   - 12 business error codes (-32001..-32011), wire-stable identifiers
//   - RFC 7807 [ProblemData] shape used as Error.Data payload
//   - [Transport] interface (Send/Recv/Close) — the SDK doesn't
//     impose a transport shape; this is our seam for HTTP / InProcess
//     / future Wails IPC.
//
// Pairing of requests with responses (matching by Message.ID) is the
// RPC client's job, not the transport's. Streaming is just an
// uninterrupted run of [Notification]s on the inbound side — no
// special framing.
package transport

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// JSONRPCVersion is the protocol version every message carries
// (JSON-RPC 2.0 spec). Re-exported for callers that need to construct
// raw envelopes; the SDK's [EncodeMessage] embeds it automatically.
const JSONRPCVersion = "2.0"

// Message is one JSON-RPC 2.0 envelope. Concrete types are
// [*Request] and [*Response]; type-switch to discriminate.
//
//   - Request with ID  → a Call
//   - Request no ID    → a Notification
//   - Response         → a Reply (Result XOR Error)
type Message = jsonrpc.Message

// Request is a Call (when ID is valid) or a Notification (when ID
// is zero). Use [Request.IsCall] to discriminate.
type Request = jsonrpc.Request

// Response is the reply to a Call. Either Result is set, or Error
// is set — never both.
type Response = jsonrpc.Response

// ID is an opaque JSON-RPC id (nil, int64, or string per spec).
// Lyra's API.md v4 §1.1 narrows this to int64 only — see
// [ValidateNumberID].
type ID = jsonrpc.ID

// Error is the JSON-RPC error envelope. The wire shape carries
// Code (int64), Message (string), Data (raw JSON — typically
// [ProblemData] per API.md §7.2).
type Error = jsonrpc.Error

// EncodeMessage serialises a Message to wire bytes (no trailing
// newline). Delegates to the SDK.
func EncodeMessage(msg Message) ([]byte, error) { return jsonrpc.EncodeMessage(msg) }

// DecodeMessage parses wire bytes into either [*Request] or
// [*Response]. Delegates to the SDK; SDK's wireCombined struct
// catches invalid envelopes (wrong version tag, malformed id).
func DecodeMessage(data []byte) (Message, error) { return jsonrpc.DecodeMessage(data) }

// NewCall builds a Request with the given ID + marshaled params.
// SDK's internal NewCall isn't re-exported through the public
// `jsonrpc` package, so we reproduce its shape here. Use a positive
// integer id; the dispatcher rejects string ids at the boundary per
// API.md v4 §1.1.
func NewCall(id int64, method string, params any) (*Request, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	jid, _ := jsonrpc.MakeID(float64(id)) // SDK uses float64 → int64 coercion
	return &Request{ID: jid, Method: method, Params: raw}, nil
}

// NewNotification builds a no-id Request — JSON-RPC Notification.
// Notifications get no response on the wire; senders are expected to
// fire-and-forget.
func NewNotification(method string, params any) (*Request, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return &Request{Method: method, Params: raw}, nil
}

// NewResponseResult builds a successful Response for the given id.
// The result is marshaled to JSON; an encoding failure surfaces as a
// CodeInternalError reply.
func NewResponseResult(id ID, result any) (*Response, error) {
	raw, err := marshalParams(result)
	if err != nil {
		return nil, err
	}
	return &Response{ID: id, Result: raw}, nil
}

// NewResponseError builds an error Response for the given id.
func NewResponseError(id ID, rpcErr *Error) *Response {
	return &Response{ID: id, Error: rpcErr}
}

// Int64ID constructs an integer JSON-RPC id. The SDK exposes only
// MakeID(any) — accepting float64 → int64 — so we wrap it.
func Int64ID(i int64) ID {
	id, _ := jsonrpc.MakeID(float64(i))
	return id
}

// marshalParams JSON-encodes a params/result value. Nil returns nil so
// the field omits on the wire.
func marshalParams(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw, nil
	}
	return json.Marshal(v)
}

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
	Send(ctx context.Context, msg Message) error

	// Recv returns the inbound channel. The channel closes when the
	// transport disconnects; consumers MUST drain it (or risk
	// goroutine leaks on the sender side).
	Recv() <-chan Message

	// Close terminates the transport. Pending Sends fail with
	// context.Canceled; Recv channel closes.
	Close() error
}

// JSON-RPC error codes — the standard band (-32700..-32603) is
// re-exported from the SDK; the Lyra business band
// (-32001..-32011) is our extension per API.md §7.2. Codes are
// stable wire identifiers; messages are advisory.
const (
	// Standard JSON-RPC errors (re-exported from SDK).
	CodeParseError     = jsonrpc.CodeParseError
	CodeInvalidRequest = jsonrpc.CodeInvalidRequest
	CodeMethodNotFound = jsonrpc.CodeMethodNotFound
	CodeInvalidParams  = jsonrpc.CodeInvalidParams
	CodeInternalError  = jsonrpc.CodeInternalError

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

// NewError builds an [Error] with the canonical message for the
// code and an optional data payload (typically [ProblemData]).
// SDK's Error.Code is int64; this helper widens our int constants.
func NewError(code int, data json.RawMessage) *Error {
	return &Error{Code: int64(code), Message: CodeMessage(code), Data: data}
}

// NewErrorWithMessage is the same as [NewError] but lets the caller
// override the message — useful when wrapping a downstream Go
// error's String() to surface its detail.
func NewErrorWithMessage(code int, msg string, data json.RawMessage) *Error {
	return &Error{Code: int64(code), Message: msg, Data: data}
}

// ProblemData mirrors RFC 7807 ProblemDetails, used as the Data
// payload on [Error] (API.md §7.2).
type ProblemData struct {
	Type         string       `json:"type,omitempty"`
	Detail       string       `json:"detail,omitempty"`
	RetryAfterMs int          `json:"retryAfterMs,omitempty"`
	Errors       []FieldError `json:"errors,omitempty"`
}

// FieldError is one entry in ProblemData.Errors — used to point at a
// specific field in invalid params.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
