package protocol

import "context"

// Lifecycle is the handshake + heartbeat surface (API.md §3).
// Initialize MUST be the first business method; the runtime rejects
// everything else until it succeeds.
type Lifecycle interface {
	// Initialize negotiates protocol version + capabilities. The
	// returned protocolVersion is what the server agrees to speak; a
	// client that cannot fall back MUST disconnect.
	Initialize(ctx context.Context, in InitializeRequest) (*InitializeResponse, error)

	// Shutdown is a notification (no wire response). Runtime stops
	// accepting new work, ends/cancels in-flight runs per host policy,
	// and closes the transport.
	Shutdown(ctx context.Context, in ShutdownRequest) error

	// Ping is a liveness probe — empty response on success. Only for
	// InProcess / IPC; HTTP probes the /v2/health sidecar (API.md §7.1).
	Ping(ctx context.Context) error
}

// InitializeRequest is the runtime.initialize request payload.
type InitializeRequest struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
	Capabilities    ClientCapabilities `json:"capabilities"`
}

// InitializeResponse is the runtime.initialize result payload.
type InitializeResponse struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

// ShutdownRequest is the runtime.shutdown notification payload.
type ShutdownRequest struct {
	Reason string `json:"reason,omitempty"`
}

// ClientInfo identifies the connecting client (logged / telemetry).
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo identifies the runtime + its serve directory context.
// Returned on initialize and on the /v2/info sidecar; cwd/home seed
// the client's cold-start default directory (API.md §3.1).
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Cwd     string `json:"cwd"`
	Home    string `json:"home"`
}
