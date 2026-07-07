package mcp

import (
	"errors"
	"strings"
)

// ErrUnknownServer is returned by [Connections.Reconnect] for a name that was
// never configured; the delivery layer maps it to invalid_params.
var ErrUnknownServer = errors.New("mcp: unknown server")

// Connection status (wire vocabulary, AUX_API §5.1). "connecting" is the
// transient reconnect state; "needsAuth" is produced when a dial fails with an
// auth-distinguishable error (a 401 / Unauthorized), so the client can prompt
// for credentials rather than treat it as a generic "failed".
const (
	statusConnected  = "connected"
	statusConnecting = "connecting"
	statusFailed     = "failed"
	statusNeedsAuth  = "needsAuth"
)

// ServerStatus is the per-server connection state exposed to
// workspace.mcp.listServers. Err is the dial / tools-list failure reason, set
// only when Status is "failed".
type ServerStatus struct {
	Name   string
	Status string
	Err    error
}

// dialStatus maps a dial error to the connection status: an
// auth-distinguishable failure becomes "needsAuth" (so the client can prompt
// for credentials), otherwise "failed".
func dialStatus(err error) string {
	if isAuthError(err) {
		return statusNeedsAuth
	}
	return statusFailed
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
