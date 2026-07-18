package mcp

import (
	"errors"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// ErrUnknownServer is returned by [Connections.Reconnect] for a name that was
// never configured; the delivery layer maps it to invalid_params.
var ErrUnknownServer = errors.New("mcp: unknown server")

// ErrConnectionsClosed reports an operation attempted after the connection
// registry began shutting down. Close is a terminal state: callers must build a
// new registry instead of reviving sessions behind the component owner's back.
var ErrConnectionsClosed = errors.New("mcp: connections closed")

// dialStatus maps a dial error to the connection status: an
// auth-distinguishable failure becomes "needsAuth" (so the client can prompt
// for credentials), otherwise "failed".
func dialStatus(err error) mcpserver.ConnectionState {
	if isAuthError(err) {
		return mcpserver.ConnectionNeedsAuth
	}
	return mcpserver.ConnectionFailed
}

// isAuthError reports whether err looks like an MCP server rejecting the
// connection for missing / invalid credentials (HTTP 401 Unauthorized). The
// go-sdk surfaces the transport failure as a wrapped error, so detection is a
// heuristic string match; a false negative just yields the generic "failed"
// status, never a wrong success.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "401") || strings.Contains(s, "unauthorized")
}
