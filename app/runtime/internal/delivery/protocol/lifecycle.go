package protocol

import "context"

// Lifecycle is the discovery surface (API.md §3). Process shutdown and request
// cancellation are host/transport concerns; liveness uses the HTTP sidecar.
type Lifecycle interface {
	// Discover returns the runtime identity and capabilities. It is a
	// stateless query, not a lifecycle transition; business methods do not
	// require it to run first.
	Discover(ctx context.Context) (*DiscoverResponse, error)
}

// DiscoverResponse is the runtime.discover result payload.
type DiscoverResponse struct {
	Protocol     ProtocolRange      `json:"protocol"`
	ServerInfo   ServerInfo         `json:"serverInfo"`
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ClientInfo identifies the connecting client (logged / telemetry).
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo identifies the runtime + its serve directory context. The full
// value is returned by runtime.discover; the public /v2/info sidecar projects
// only name and version. Cwd/Home seed the client's cold-start directory.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Cwd     string `json:"cwd"`
	Home    string `json:"home"`
}
