package protocol

import "context"

// Lifecycle is the discovery + heartbeat surface (API.md §3).
type Lifecycle interface {
	// Discover returns the runtime identity and capabilities. It is a
	// stateless query, not a lifecycle transition; business methods do not
	// require it to run first.
	Discover(ctx context.Context) (*DiscoverResponse, error)

	// Shutdown is a notification (no wire response). Runtime stops
	// accepting new work, ends/cancels in-flight runs per host policy,
	// and closes the transport.
	Shutdown(ctx context.Context, in ShutdownRequest) error

	// Ping is a liveness probe — empty response on success. Only for
	// InProcess / IPC; HTTP probes the /v2/health sidecar (API.md §7.1).
	Ping(ctx context.Context) error
}

// DiscoverResponse is the runtime.discover result payload.
type DiscoverResponse struct {
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
// Returned by runtime.discover and the /v2/info sidecar; cwd/home seed
// the client's cold-start default directory (API.md §3.1).
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Cwd     string `json:"cwd"`
	Home    string `json:"home"`
}
