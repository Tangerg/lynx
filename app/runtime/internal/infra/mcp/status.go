package mcp

import (
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// ErrUnknownServer is returned by [Connections.Reconnect] for a name that was
// never configured; the delivery layer maps it to invalid_params.
var ErrUnknownServer = errors.New("mcp: unknown server")

// ErrConnectionsClosed reports an operation attempted after the connection
// registry began shutting down. Close is a terminal state: callers must build a
// new registry instead of reviving sessions behind the component owner's back.
var ErrConnectionsClosed = errors.New("mcp: connections closed")

type dialFailureKind uint8

const dialFailureNeedsAuth dialFailureKind = iota + 1

type dialFailure struct {
	kind dialFailureKind
	err  error
}

func (e *dialFailure) Error() string { return e.err.Error() }
func (e *dialFailure) Unwrap() error { return e.err }

// dialStatus maps a dial error to the connection status: an
// auth-distinguishable failure becomes "needsAuth" (so the client can prompt
// for credentials), otherwise "failed".
func dialStatus(err error) mcpserver.ConnectionState {
	if isAuthError(err) {
		return mcpserver.ConnectionNeedsAuth
	}
	return mcpserver.ConnectionFailed
}

// isAuthError reports whether the HTTP transport observed an authentication
// rejection while the MCP SDK was dialing. The SDK turns the response into a
// plain wrapped error, so our transport records the status before that type
// information is lost.
func isAuthError(err error) bool {
	var failure *dialFailure
	return errors.As(err, &failure) && failure.kind == dialFailureNeedsAuth
}
