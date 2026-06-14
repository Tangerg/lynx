// Package transport is the Lyra Runtime Protocol's transport layer
// — a bidirectional pipe for JSON-RPC 2.0 messages. See
// docs/TRANSPORT.md for the architectural picture.
//
// One [Transport] interface, two implementations:
//
//   - rpc/transport/inprocess — Go ↔ Go in the same binary, business
//     path bypasses JSON serialization entirely.
//   - rpc/transport/http      — JSON-RPC over streamable HTTP (POST
//     /v2/rpc/{method}; a streaming method's events ride its own
//     text/event-stream response) + sidecar /v2/info, /v2/health.
//
// Wire envelope types and encode/decode are re-exported from the MCP
// Go SDK's `jsonrpc` package — same vendor we use for our MCP
// integration, conformant JSON-RPC 2.0 implementation, "for use by
// mcp transport authors" per its own doc.
//
// File layout in this package:
//   - transport.go (this file) — [Transport] interface + Message/
//     Request/Response/ID/Error type aliases
//   - codes.go    — wire error codes + canonical-message helper
//   - builders.go — NewCall/NewNotification/NewResponseResult/
//     NewResponseError + StringID convenience constructors
package transport

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

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
// Lyra's API.md §1 narrows this to string only — the dispatcher
// rejects non-string ids at the boundary.
type ID = jsonrpc.ID

// Error is the JSON-RPC error envelope. The wire shape carries
// Code (int64), Message (string), Data (raw JSON — typically
// [ProblemData] per API.md §8).
type Error = jsonrpc.Error

// EncodeMessage serializes a Message to wire bytes (no trailing
// newline). Delegates to the SDK.
func EncodeMessage(msg Message) ([]byte, error) { return jsonrpc.EncodeMessage(msg) }

// DecodeMessage parses wire bytes into either [*Request] or
// [*Response]. Delegates to the SDK; SDK's wireCombined struct
// catches invalid envelopes (wrong version tag, malformed id).
func DecodeMessage(data []byte) (Message, error) { return jsonrpc.DecodeMessage(data) }

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
