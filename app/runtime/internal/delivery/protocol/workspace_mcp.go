package protocol

// MCPServerRequest identifies a configured MCP server by name.
type MCPServerRequest struct {
	Server string `json:"server"`
}

// RemoveMCPServerRequest identifies the MCP configuration to remove.
type RemoveMCPServerRequest struct {
	Name string `json:"name"`
}

// MCPListToolsRequest — workspace.mcp.listTools body.
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
// configuration (workspace.mcp.listConfigs / configure), distinct from McpServer
// (the live status from listServers). The bearer token is returned masked. Live
// connection state (status / toolCount / error) is intentionally NOT carried
// here — read it from workspace.mcp.listServers (McpServer), keyed by name, so
// the editable and observed shapes don't cross-contaminate.
type McpServerConfig struct {
	Name                string            `json:"name"`
	Transport           string            `json:"type"` // "stdio" | "streamableHttp" (standard mcpServers vocab)
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

// ConfigureMCPServerRequest — workspace.mcp.configure / test body (the editable
// fields of McpServerConfig). Authorization is the RAW bearer token (http only);
// an empty Authorization when (re)configuring or testing an EXISTING server
// preserves its stored token, so editing other fields needn't re-enter the
// secret — clear a token by removing the server, not by blanking it.
type ConfigureMCPServerRequest struct {
	Name             string            `json:"name"`
	Transport        string            `json:"type"`
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

// SetMCPEnabledRequest — workspace.mcp.setEnabled body.
type SetMCPEnabledRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// McpTestResult — workspace.mcp.test result (a connection probe; mirrors
// ProviderTestResult).
type McpTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}
