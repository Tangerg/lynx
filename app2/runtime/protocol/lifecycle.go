package protocol

// DiscoverResponse is the runtime.discover result payload.
type DiscoverResponse struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

// ClientInfo identifies the connecting client (logged / telemetry).
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo identifies one Runtime serving incarnation and its serve directory
// context. InstanceID is fresh for every Bootstrap instance (one per standalone
// process) and must never be used as a durable storage or idempotency namespace.
// The full value is returned by runtime.discover; the public /v2/info sidecar
// projects only public process identity. DefaultWorkspace/Home seed the client's
// cold-start filesystem context.
type ServerInfo struct {
	InstanceID       string       `json:"instanceId"`
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	DefaultWorkspace WorkspaceRef `json:"defaultWorkspace"`
	Home             string       `json:"home"`
}
