package integrations

import (
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/secretmask"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// MCPServerInput is the application command for configuring or testing an MCP
// server. Authorization is write-only: Application consumes it to persist or
// probe a connection, but no application read model returns the raw secret.
type MCPServerInput struct {
	Name             string
	Transport        mcpserver.Transport
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
	Transport           mcpserver.Transport
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

// MCPServerStatus is the application status read model. Known is false after a
// removed server's final notification; Delivery then emits the protocol's bare
// deletion event without re-querying live infrastructure. Connection state is
// a domain fact, so application does not recreate a second state vocabulary.
type MCPServerStatus struct {
	Name      string
	Known     bool
	State     mcpserver.ConnectionState
	ToolCount *int
}

// MCPTestResult is the semantic outcome of a non-persisting connection probe.
// Delivery maps a failed probe to its client-facing safe diagnostic.
type MCPTestResult struct {
	OK bool
}

func (in MCPServerInput) server() mcpserver.Server {
	return mcpserver.Server{
		Name:             in.Name,
		Transport:        in.Transport,
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
		Transport:           server.Transport,
		Enabled:             server.Enabled,
		Description:         server.Description,
		URL:                 server.URL,
		AuthorizationMasked: secretmask.Mask(server.Authorization),
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
	return MCPServerStatus{Name: status.Name, Known: true, State: status.State, ToolCount: toolCount}
}
