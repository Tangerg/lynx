package integrations

import (
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/secret"
)

// MCPServerInput is the application command for configuring or testing an MCP
// server. Authorization is write-only: Application consumes it to persist or
// probe a connection, but no application read model returns the raw secret.
type MCPServerInput struct {
	Name             string
	Transport        string
	Enabled          bool
	Description      string
	URL              string
	Authorization    string
	Headers          map[string]string
	Command          string
	Args             []string
	Env              map[string]string
	Dir              string
	Timeout          time.Duration
	DisabledTools    []string
	AutoApproveTools []string
}

// MCPServerConfig is the safe application read model for an editable MCP
// configuration. Secrets are represented only by their redacted form.
type MCPServerConfig struct {
	Name                string
	Transport           string
	Enabled             bool
	Description         string
	URL                 string
	AuthorizationMasked string
	Headers             map[string]string
	Command             string
	Args                []string
	Env                 map[string]string
	Dir                 string
	Timeout             time.Duration
	DisabledTools       []string
	AutoApproveTools    []string
}

// MCPConnectionState is the application-owned status vocabulary exposed to
// outer adapters. The MCP domain's live state remains an internal source fact.
type MCPConnectionState string

const (
	MCPConnecting MCPConnectionState = "connecting"
	MCPConnected  MCPConnectionState = "connected"
	MCPFailed     MCPConnectionState = "failed"
	MCPNeedsAuth  MCPConnectionState = "needsAuth"
)

// MCPProblem is a safe diagnostic suitable for presentation. It deliberately
// contains no adapter error because those errors can expose endpoint, process,
// or credential details.
type MCPProblem struct {
	Type   string
	Detail string
}

// MCPServerStatus is a fully resolved application status read model. Known is
// false after a removed server's final notification; Delivery then emits the
// protocol's bare deletion event without re-querying live infrastructure.
type MCPServerStatus struct {
	Name      string
	Known     bool
	State     MCPConnectionState
	ToolCount *int
	Problem   *MCPProblem
}

// MCPTestResult is the safe outcome of a non-persisting connection probe.
type MCPTestResult struct {
	OK      bool
	Problem *MCPProblem
}

func (in MCPServerInput) server() mcpserver.Server {
	return mcpserver.Server{
		Name:             in.Name,
		Transport:        mcpserver.Transport(in.Transport),
		Enabled:          in.Enabled,
		Description:      in.Description,
		URL:              in.URL,
		Authorization:    in.Authorization,
		Headers:          maps.Clone(in.Headers),
		Command:          in.Command,
		Args:             slices.Clone(in.Args),
		Env:              maps.Clone(in.Env),
		Dir:              in.Dir,
		Timeout:          in.Timeout,
		DisabledTools:    slices.Clone(in.DisabledTools),
		AutoApproveTools: slices.Clone(in.AutoApproveTools),
	}
}

func mcpConfigView(server mcpserver.Server) MCPServerConfig {
	return MCPServerConfig{
		Name:                server.Name,
		Transport:           string(server.Transport),
		Enabled:             server.Enabled,
		Description:         server.Description,
		URL:                 server.URL,
		AuthorizationMasked: secret.Mask(server.Authorization),
		Headers:             maps.Clone(server.Headers),
		Command:             server.Command,
		Args:                slices.Clone(server.Args),
		Env:                 maps.Clone(server.Env),
		Dir:                 server.Dir,
		Timeout:             server.Timeout,
		DisabledTools:       slices.Clone(server.DisabledTools),
		AutoApproveTools:    slices.Clone(server.AutoApproveTools),
	}
}

func mcpStatusView(status mcpserver.ConnectionStatus, toolCount *int) MCPServerStatus {
	view := MCPServerStatus{Name: status.Name, Known: true, ToolCount: toolCount}
	switch status.State {
	case mcpserver.ConnectionConnecting:
		view.State = MCPConnecting
	case mcpserver.ConnectionConnected:
		view.State = MCPConnected
	case mcpserver.ConnectionNeedsAuth:
		view.State = MCPNeedsAuth
		view.Problem = &MCPProblem{Type: "mcp_authorization_required", Detail: "MCP authorization is required."}
	case mcpserver.ConnectionFailed:
		view.State = MCPFailed
		view.Problem = &MCPProblem{Type: "mcp_dial_failed", Detail: "MCP connection failed."}
	default:
		view.State = MCPFailed
		view.Problem = &MCPProblem{Type: "mcp_invalid_connection_state", Detail: "Runtime reported an unknown MCP connection state."}
	}
	return view
}
