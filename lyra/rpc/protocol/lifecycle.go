package protocol

import "context"

// Lifecycle is the handshake + heartbeat surface (API.md §2).
// Initialize MUST be the first business method called; the runtime
// rejects everything else with -32011 protocol_violation until it
// succeeds.
type Lifecycle interface {
	// Initialize negotiates protocol version + capabilities. The
	// returned protocolVersion is what the server agrees to speak;
	// when the client cannot fall back to it, the client MUST
	// disconnect.
	Initialize(ctx context.Context, in InitializeRequest) (*InitializeResponse, error)

	// Shutdown is a polite "I'm leaving" notification (no response
	// expected on the wire — but the impl returns error for parity).
	// Runtime stops accepting new requests, cancels in-flight runs
	// with notifications/cancelled, and closes the transport.
	Shutdown(ctx context.Context, in ShutdownRequest) error

	// Ping is a liveness probe — empty response on success. Most
	// transports prefer the sidecar /v1/health endpoint when they
	// want to probe without going through initialize.
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

// ClientInfo identifies the connecting client. Logged + surfaced to
// telemetry; doesn't drive business logic.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo identifies the runtime. Returned on initialize and on
// the /v1/info sidecar.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
