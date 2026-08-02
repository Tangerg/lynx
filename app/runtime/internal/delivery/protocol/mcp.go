package protocol

import (
	"context"
	"time"
)

// MCP is the mcp.* resource group. One McpServer resource carries both the
// durable configuration and its current connection state; clients never join a
// second configuration collection with a transient status collection.
type MCP interface {
	ListMCPServers(ctx context.Context, q PageQuery) (*Page[McpServer], error)
	CreateMCPServer(ctx context.Context, in CreateMCPServerRequest) (*McpServer, error)
	UpdateMCPServer(ctx context.Context, in UpdateMCPServerRequest) (*McpServer, error)
	DeleteMCPServer(ctx context.Context, server string) error
	TestMCPServer(ctx context.Context, in MCPServerCandidate) (*McpTestResult, error)
	ListMCPTools(ctx context.Context, in MCPListToolsRequest) (*Page[McpTool], error)
	ReconnectMCPServer(ctx context.Context, server string) error
	CreateMCPAuthorizationAttempt(ctx context.Context, server string) (*McpAuthorizationAttempt, error)
	GetMCPAuthorizationAttempt(ctx context.Context, attemptID string) (*McpAuthorizationAttempt, error)
}

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
	PageQuery
}

// McpServer is the single safe read model for one configured MCP server. Its
// status includes "disabled", so configuration enablement and live lifecycle
// can never contradict one another on the wire.
type McpServer struct {
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Connection       McpConnection  `json:"connection"`
	TimeoutSeconds   int            `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string       `json:"disabledTools,omitempty"`
	AutoApproveTools []string       `json:"autoApproveTools,omitempty"`
	Status           McpServerState `json:"status"`
}

// McpServerStateType is the complete lifecycle of a configured MCP server. A
// disabled entry is durable but intentionally has no live connection.
type McpServerStateType string

const (
	McpServerDisabled     McpServerStateType = "disabled"
	McpServerDisconnected McpServerStateType = "disconnected"
	McpServerConnecting   McpServerStateType = "connecting"
	McpServerConnected    McpServerStateType = "connected"
	McpServerFailed       McpServerStateType = "failed"
	McpServerNeedsAuth    McpServerStateType = "needsAuth"
)

// McpServerState is a closed union. toolCount belongs only to connected;
// error belongs only to failed and needsAuth.
type McpServerState struct {
	Type      McpServerStateType `json:"type"`
	ToolCount *int               `json:"toolCount,omitempty"`
	Error     *ProblemData       `json:"error,omitempty"`
}

// McpTransport is the protocol's closed MCP transport vocabulary.
type McpTransport string

const (
	McpTransportStdio          McpTransport = "stdio"
	McpTransportStreamableHTTP McpTransport = "streamableHttp"
)

// McpConnection is the safe output union for a server's connection descriptor.
// Secret-bearing values are write-only; reads expose only masked representations.
type McpConnection struct {
	Type                McpTransport      `json:"type"`
	URL                 string            `json:"url,omitempty"`
	AuthorizationMasked string            `json:"authorizationMasked,omitempty"`
	HeadersMasked       map[string]string `json:"headersMasked,omitempty"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	EnvMasked           map[string]string `json:"envMasked,omitempty"`
	Dir                 string            `json:"dir,omitempty"`
}

// McpSecretChangeType gives a secret update exact three-state semantics:
// omission preserves, set replaces, and clear removes.
type McpSecretChangeType string

const (
	McpSecretSet   McpSecretChangeType = "set"
	McpSecretClear McpSecretChangeType = "clear"
)

// McpAuthorizationChange is the write-only authorization change union.
type McpAuthorizationChange struct {
	Type  McpSecretChangeType `json:"type"`
	Value string              `json:"value,omitempty"`
}

// McpHeadersChange is the write-only full replacement for HTTP headers. Header
// values may contain credentials, so reads expose masked values and updates use
// the same exact omission/set/clear semantics as Authorization.
type McpHeadersChange struct {
	Type  McpSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

// McpEnvironmentChange is the write-only full replacement for a stdio
// process's environment. Environment values may contain credentials, so reads
// expose masked values and updates use exact omission/set/clear semantics.
type McpEnvironmentChange struct {
	Type  McpSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

// McpConnectionInput is the write union for a complete connection descriptor.
// A connection replacement is atomic: fields from the other transport cannot
// survive a transport switch.
type McpConnectionInput struct {
	Type          McpTransport            `json:"type"`
	URL           string                  `json:"url,omitempty"`
	Authorization *McpAuthorizationChange `json:"authorization,omitempty"`
	Headers       *McpHeadersChange       `json:"headers,omitempty"`
	Command       string                  `json:"command,omitempty"`
	Args          []string                `json:"args,omitempty"`
	Env           *McpEnvironmentChange   `json:"env,omitempty"`
	Dir           string                  `json:"dir,omitempty"`
}

// MCPServerCandidate is a complete, unpersisted MCP server descriptor. Create
// persists it; test probes it without changing durable or live state.
type MCPServerCandidate struct {
	Name             string             `json:"name"`
	Enabled          bool               `json:"enabled"`
	Description      string             `json:"description,omitempty"`
	Connection       McpConnectionInput `json:"connection"`
	TimeoutSeconds   int                `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string           `json:"disabledTools,omitempty"`
	AutoApproveTools []string           `json:"autoApproveTools,omitempty"`
}

// CreateMCPServerRequest — mcp.servers.create body.
type CreateMCPServerRequest = MCPServerCandidate

// UpdateMCPServerRequest — mcp.servers.update body. Omitted members preserve
// their current value; present empty strings, collections, and zeroes clear it.
// Name is immutable and addressed by Server.
type UpdateMCPServerRequest struct {
	Server           string              `json:"server"`
	Enabled          *bool               `json:"enabled,omitempty"`
	Description      *string             `json:"description,omitempty"`
	Connection       *McpConnectionInput `json:"connection,omitempty"`
	TimeoutSeconds   *int                `json:"timeoutSeconds,omitempty"`
	DisabledTools    *[]string           `json:"disabledTools,omitempty"`
	AutoApproveTools *[]string           `json:"autoApproveTools,omitempty"`
}

// McpTool is one tool exposed by an MCP server (API.md §4.10).
type McpTool struct {
	Server      string         `json:"server"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// McpTestResult is the semantic result of mcp.servers.test.
type McpTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}

// McpAuthorizationAttemptStatusType is the complete lifecycle of one
// interactive MCP OAuth flow.
type McpAuthorizationAttemptStatusType string

const (
	McpAuthorizationAttemptPending   McpAuthorizationAttemptStatusType = "pending"
	McpAuthorizationAttemptSucceeded McpAuthorizationAttemptStatusType = "succeeded"
	McpAuthorizationAttemptFailed    McpAuthorizationAttemptStatusType = "failed"
	McpAuthorizationAttemptCanceled  McpAuthorizationAttemptStatusType = "canceled"
)

// McpAuthorizationAttemptStatus is a closed union. Only failed carries an
// error; the full provider/OAuth error remains private telemetry.
type McpAuthorizationAttemptStatus struct {
	Type  McpAuthorizationAttemptStatusType `json:"type"`
	Error *ProblemData                      `json:"error,omitempty"`
}

// McpAuthorizationAttempt is the observable asynchronous result of interactive
// authorization. Pending has no finishedAt; every terminal status has one.
type McpAuthorizationAttempt struct {
	ID         string                        `json:"id"`
	Server     string                        `json:"server"`
	Status     McpAuthorizationAttemptStatus `json:"status"`
	CreatedAt  time.Time                     `json:"createdAt"`
	FinishedAt *time.Time                    `json:"finishedAt,omitempty"`
}
