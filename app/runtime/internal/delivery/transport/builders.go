package transport

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// Message constructors — the MCP SDK's public `jsonrpc` package doesn't
// re-export the internal `NewCall` / `NewNotification` / `NewResponse`
// helpers. We reproduce them here using the public type aliases so callers
// don't have to construct envelopes by hand or reach into internal/jsonrpc2.

// NewCall builds a Request with the given string ID + marshaled params.
// API.md §1.1: envelope ids are strings (the dispatcher rejects
// non-string ids at the boundary).
func NewCall(id string, method string, params any) (*Request, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return &Request{ID: StringID(id), Method: method, Params: raw}, nil
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

// NewError builds an RPC error with a caller-selected message and structured
// data — useful when a downstream error's detail is safe to surface.
func NewError(code int, msg string, data json.RawMessage) *Error {
	return &Error{Code: int64(code), Message: msg, Data: data}
}

// StringID constructs a string JSON-RPC id (API.md §1.1 — all envelope
// ids are strings). The SDK exposes only MakeID(any); we wrap it.
func StringID(s string) ID {
	id, _ := jsonrpc.MakeID(s)
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
