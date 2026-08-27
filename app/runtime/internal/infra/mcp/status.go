package mcp

import (
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// ErrUnknownServer is returned by [Connections.Reconnect] for a name that was
// never configured. Callers can distinguish it from a configured server whose
// connection attempt failed.
var ErrUnknownServer = errors.New("mcp: unknown server")

// ErrConnectionsClosed reports an operation attempted after the connection
// registry began shutting down. Shutdown is a terminal state: callers must build a
// new registry instead of reviving sessions behind the component owner's back.
var ErrConnectionsClosed = errors.New("mcp: connections closed")

// ErrConnectionsUnavailable reports a missing live connection pool. Mutating a
// nil pool is a composition error, never a successful no-op.
var ErrConnectionsUnavailable = errors.New("mcp: connections unavailable")

type dialErrorKind uint8

const dialErrorNeedsAuth dialErrorKind = iota + 1

type dialError struct {
	kind dialErrorKind
	err  error
}

func (d *dialError) Error() string { return d.err.Error() }
func (d *dialError) Unwrap() error { return d.err }

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
	failure, ok := errors.AsType[*dialError](err)
	return ok && failure.kind == dialErrorNeedsAuth
}
