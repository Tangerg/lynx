package mcpserver

import "errors"

// ConnectionState is the lifecycle state of a configured MCP connection.
// Keeping this vocabulary canonical prevents subtly different values for the
// same user-visible fact.
type ConnectionState string

const (
	ConnectionConnecting ConnectionState = "connecting"
	ConnectionConnected  ConnectionState = "connected"
	ConnectionFailed     ConnectionState = "failed"
	ConnectionNeedsAuth  ConnectionState = "needsAuth"
)

// ConnectionStatus is the safe, per-server live projection exposed by the MCP
// control plane. Connection failures stay in the operation and observability
// paths; a status is deliberately not an error transport.
type ConnectionStatus struct {
	Name      string
	State     ConnectionState
	ToolCount int
}

// ErrUnknownServer is returned when a live MCP operation addresses a server
// that was never configured.
var ErrUnknownServer = errors.New("mcp: unknown server")

// AdvertisedTool is one tool advertised by a connected MCP server.
type AdvertisedTool struct {
	Server      string
	Name        string
	Description string
	InputSchema InputSchema
}
