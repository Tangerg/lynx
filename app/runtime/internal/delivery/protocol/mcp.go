package protocol

import "context"

// MCP is the mcp.* method group. Server status, exposed tools, and editable
// configuration are one integration surface even though they use sub-roots.
type MCP interface {
	ListMCPServers(ctx context.Context, q PageQuery) (*Page[McpServer], error)
	ListMCPTools(ctx context.Context, in MCPListToolsRequest) (*Page[McpTool], error)
	ReconnectMCPServer(ctx context.Context, server string) error
	AuthorizeMCPServer(ctx context.Context, server string) error
	ListMCPServerConfigs(ctx context.Context, q PageQuery) (*Page[McpServerConfig], error)
	ConfigureMCPServer(ctx context.Context, in ConfigureMCPServerRequest) (*McpServerConfig, error)
	RemoveMCPServer(ctx context.Context, name string) error
	SetMCPServerEnabled(ctx context.Context, in SetMCPEnabledRequest) error
	TestMCPServer(ctx context.Context, in ConfigureMCPServerRequest) (*McpTestResult, error)
}

// MCPServerRequest identifies a configured MCP server by name.
type MCPServerRequest struct {
	Server string `json:"server"`
}

// RemoveMCPServerRequest identifies the MCP configuration to remove.
type RemoveMCPServerRequest struct {
	Name string `json:"name"`
}

// MCPListToolsRequest — mcp.tools.list body.
type MCPListToolsRequest struct {
	Server string `json:"server,omitempty"`
	PageQuery
}

// McpStatus is an MCP server's connection state (AUX_API §5.1). Carried on
// McpServer.Status and the mcp.serverChanged WorkspaceEvent.
type McpStatus string

const (
	McpConnecting   McpStatus = "connecting"
	McpConnected    McpStatus = "connected"
	McpDisconnected McpStatus = "disconnected"
	McpFailed       McpStatus = "failed"
	McpNeedsAuth    McpStatus = "needsAuth"
)

// McpAuthStatus is an MCP server's auth posture (AUX_API §5.1); omitted when
// the server tracks no auth.
type McpAuthStatus string

const (
	McpAuthNone        McpAuthStatus = "none"
	McpAuthBearerToken McpAuthStatus = "bearerToken"
	McpAuthOAuth       McpAuthStatus = "oauth"
	McpAuthNotLoggedIn McpAuthStatus = "notLoggedIn"
)

// McpServer is one configured MCP server (API.md §4.10).
type McpServer struct {
	Name       string        `json:"name"`
	Status     McpStatus     `json:"status"` // see McpStatus
	ToolCount  *int          `json:"toolCount,omitempty"`
	AuthStatus McpAuthStatus `json:"authStatus,omitempty"` // see McpAuthStatus; omitted when untracked
	// Error carries the reason for a failed server (AUX_API §5.1); set only
	// when Status is "failed".
	Error       *ProblemData `json:"error,omitempty"`
	Description string       `json:"description,omitempty"`
}

// McpTool is one tool exposed by an MCP server (API.md §4.10).
type McpTool struct {
	Server      string         `json:"server"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// McpServerConfig is one entry in the MCP-server registry — the editable
// configuration (mcp.configs.list / configure), distinct from McpServer
// (the live status from listServers). The bearer token is returned masked. Live
// connection state (status / toolCount / error) is intentionally NOT carried
// here — read it from mcp.servers.list (McpServer), keyed by name, so
// the editable and observed shapes don't cross-contaminate.
type McpServerConfig struct {
	Name                string            `json:"name"`
	Transport           McpTransport      `json:"type"`
	Enabled             bool              `json:"enabled"`
	Description         string            `json:"description,omitempty"`
	URL                 string            `json:"url,omitempty"`                 // http transport
	AuthorizationMasked string            `json:"authorizationMasked,omitempty"` // http; "" = none
	Headers             map[string]string `json:"headers,omitempty"`             // http; extra request headers (not masked)
	Command             string            `json:"command,omitempty"`             // stdio transport
	Args                []string          `json:"args,omitempty"`
	Env                 map[string]string `json:"env,omitempty"` // stdio; KEY→value, replaces subprocess env
	Dir                 string            `json:"dir,omitempty"`
	TimeoutSeconds      int               `json:"timeoutSeconds,omitempty"`   // connect-handshake bound; 0 = unbounded
	DisabledTools       []string          `json:"disabledTools,omitempty"`    // hidden from the model
	AutoApproveTools    []string          `json:"autoApproveTools,omitempty"` // skip the approval gate
}

// McpTransport is the protocol's closed MCP transport vocabulary.
type McpTransport string

const (
	McpTransportStdio          McpTransport = "stdio"
	McpTransportStreamableHTTP McpTransport = "streamableHttp"
)

// ConfigureMCPServerRequest — mcp.configs.configure / test body (the editable
// fields of McpServerConfig). Authorization is the RAW bearer token (http only);
// an empty Authorization when (re)configuring or testing an EXISTING server
// preserves its stored token only while the HTTP endpoint origin is unchanged,
// so editing other fields needn't re-enter the secret without allowing a URL
// change to transfer credentials to another origin.
type ConfigureMCPServerRequest struct {
	Name             string            `json:"name"`
	Transport        McpTransport      `json:"type"`
	Enabled          bool              `json:"enabled"`
	Description      string            `json:"description,omitempty"`
	URL              string            `json:"url,omitempty"`
	Authorization    string            `json:"authorization,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Command          string            `json:"command,omitempty"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Dir              string            `json:"dir,omitempty"`
	TimeoutSeconds   int               `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string          `json:"disabledTools,omitempty"`
	AutoApproveTools []string          `json:"autoApproveTools,omitempty"`
}

// SetMCPEnabledRequest — mcp.configs.setEnabled body.
type SetMCPEnabledRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// McpTestResult — mcp.configs.test result (a connection probe; mirrors
// ProviderTestResult).
type McpTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}
