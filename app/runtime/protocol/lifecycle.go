package protocol

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
// only name and version. DefaultWorkspace/Home seed the client's cold-start
// filesystem context.
type ServerInfo struct {
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	DefaultWorkspace WorkspaceRef `json:"defaultWorkspace"`
	Home             string       `json:"home"`
}
