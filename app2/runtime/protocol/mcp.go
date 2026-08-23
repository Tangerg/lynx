package protocol

import (
	"time"
)

// MCPServerRequest identifies a configured MCP server by its stable name.
type MCPServerRequest struct {
	Server string `json:"server"`
}

// CreateMCPAuthorizationAttemptRequest starts one interactive OAuth flow for a
// configured server.
type CreateMCPAuthorizationAttemptRequest struct {
	Server string `json:"server"`
}

// MCPAuthorizationAttemptRequest identifies one interactive OAuth flow.
type MCPAuthorizationAttemptRequest struct {
	AttemptID string `json:"attemptId"`
}

// MCPListToolsRequest — mcp.tools.list body.
type MCPListToolsRequest struct {
	Server string `json:"server,omitempty"`
}

// MCPServer is the single safe read model for one configured MCP server. Its
// status includes "disabled", so configuration enablement and live lifecycle
// can never contradict one another on the wire.
type MCPServer struct {
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Connection       MCPConnection  `json:"connection"`
	TimeoutSeconds   int            `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string       `json:"disabledTools,omitempty"`
	AutoApproveTools []string       `json:"autoApproveTools,omitempty"`
	Status           MCPServerState `json:"status"`
}

// MCPServerStateType is the complete lifecycle of a configured MCP server. A
// disabled entry is durable but intentionally has no live connection.
type MCPServerStateType string

const (
	MCPServerDisabled     MCPServerStateType = "disabled"
	MCPServerDisconnected MCPServerStateType = "disconnected"
	MCPServerConnecting   MCPServerStateType = "connecting"
	MCPServerConnected    MCPServerStateType = "connected"
	MCPServerFailed       MCPServerStateType = "failed"
	MCPServerNeedsAuth    MCPServerStateType = "needsAuth"
)

// MCPServerState is a closed union. toolCount belongs only to connected;
// error belongs only to failed and needsAuth.
type MCPServerState struct {
	Type      MCPServerStateType `json:"type"`
	ToolCount *int               `json:"toolCount,omitempty"`
	Error     *ProblemData       `json:"error,omitempty"`
}

// MCPTransport is the protocol's closed MCP transport vocabulary.
type MCPTransport string

const (
	MCPTransportStdio          MCPTransport = "stdio"
	MCPTransportStreamableHTTP MCPTransport = "streamableHttp"
)

// MCPConnection is the safe output union for a server's connection descriptor.
// Secret-bearing values are write-only; reads expose only masked representations.
type MCPConnection struct {
	Type                MCPTransport      `json:"type"`
	URL                 string            `json:"url,omitempty"`
	AuthorizationMasked string            `json:"authorizationMasked,omitempty"`
	HeadersMasked       map[string]string `json:"headersMasked,omitempty"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	EnvMasked           map[string]string `json:"envMasked,omitempty"`
	Dir                 string            `json:"dir,omitempty"`
}

// MCPSecretChangeType gives a secret update exact three-state semantics:
// omission preserves, set replaces, and clear removes.
type MCPSecretChangeType string

const (
	MCPSecretSet   MCPSecretChangeType = "set"
	MCPSecretClear MCPSecretChangeType = "clear"
)

// MCPAuthorizationChange is the write-only authorization change union.
type MCPAuthorizationChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value string              `json:"value,omitempty"`
}

// MCPHeadersChange is the write-only full replacement for HTTP headers. Header
// values may contain credentials, so reads expose masked values and updates use
// the same exact omission/set/clear semantics as Authorization.
type MCPHeadersChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

// MCPEnvironmentChange is the write-only full replacement for a stdio
// process's environment. Environment values may contain credentials, so reads
// expose masked values and updates use exact omission/set/clear semantics.
type MCPEnvironmentChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

// MCPConnectionInput is the write union for a complete connection descriptor.
// A connection replacement is atomic: fields from the other transport cannot
// survive a transport switch.
type MCPConnectionInput struct {
	Type          MCPTransport            `json:"type"`
	URL           string                  `json:"url,omitempty"`
	Authorization *MCPAuthorizationChange `json:"authorization,omitempty"`
	Headers       *MCPHeadersChange       `json:"headers,omitempty"`
	Command       string                  `json:"command,omitempty"`
	Args          []string                `json:"args,omitempty"`
	Env           *MCPEnvironmentChange   `json:"env,omitempty"`
	Dir           string                  `json:"dir,omitempty"`
}

// MCPServerCandidate is a complete, unpersisted MCP server descriptor. Create
// persists it; test probes it without changing durable or live state.
type MCPServerCandidate struct {
	Name             string             `json:"name"`
	Enabled          bool               `json:"enabled"`
	Description      string             `json:"description,omitempty"`
	Connection       MCPConnectionInput `json:"connection"`
	TimeoutSeconds   int                `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string           `json:"disabledTools,omitempty"`
	AutoApproveTools []string           `json:"autoApproveTools,omitempty"`
}

// UpdateMCPServerRequest — mcp.servers.update body. Omitted members preserve
// their current value; present empty strings, collections, and zeroes clear it.
// Name is immutable and addressed by Server.
type UpdateMCPServerRequest struct {
	Server           string              `json:"server"`
	Enabled          *bool               `json:"enabled,omitempty"`
	Description      *string             `json:"description,omitempty"`
	Connection       *MCPConnectionInput `json:"connection,omitempty"`
	TimeoutSeconds   *int                `json:"timeoutSeconds,omitempty"`
	DisabledTools    *[]string           `json:"disabledTools,omitempty"`
	AutoApproveTools *[]string           `json:"autoApproveTools,omitempty"`
}

// MCPTool is one tool exposed by an MCP server (API.md §4.10).
type MCPTool struct {
	Server      string         `json:"server"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// MCPTestResult is the semantic result of mcp.servers.test.
type MCPTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}

// MCPAuthorizationAttemptStatusType is the complete lifecycle of one
// interactive MCP OAuth flow.
type MCPAuthorizationAttemptStatusType string

const (
	MCPAuthorizationAttemptPending   MCPAuthorizationAttemptStatusType = "pending"
	MCPAuthorizationAttemptSucceeded MCPAuthorizationAttemptStatusType = "succeeded"
	MCPAuthorizationAttemptFailed    MCPAuthorizationAttemptStatusType = "failed"
	MCPAuthorizationAttemptCanceled  MCPAuthorizationAttemptStatusType = "canceled"
)

// MCPAuthorizationAttemptStatus is a closed union. Only failed carries an
// error; the full provider/OAuth error remains private telemetry.
type MCPAuthorizationAttemptStatus struct {
	Type  MCPAuthorizationAttemptStatusType `json:"type"`
	Error *ProblemData                      `json:"error,omitempty"`
}

// MCPAuthorizationAttempt is the observable asynchronous result of interactive
// authorization. Pending has no finishedAt; every terminal status has one.
type MCPAuthorizationAttempt struct {
	ID         string                        `json:"id"`
	Server     string                        `json:"server"`
	Status     MCPAuthorizationAttemptStatus `json:"status"`
	CreatedAt  time.Time                     `json:"createdAt"`
	FinishedAt *time.Time                    `json:"finishedAt,omitempty"`
}
