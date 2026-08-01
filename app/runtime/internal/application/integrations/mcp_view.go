package integrations

import (
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/secretmask"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// MCPSecretChangeKind is the application's exact secret-mutation
// vocabulary. Delivery translates the wire union into it; persistence never
// has to infer intent from an empty string.
type MCPSecretChangeKind uint8

const (
	MCPSecretSet MCPSecretChangeKind = iota + 1
	MCPSecretClear
)

// MCPAuthorizationChange is a write-only bearer-token mutation.
type MCPAuthorizationChange struct {
	Kind  MCPSecretChangeKind
	Value string
}

// MCPHeadersChange is a write-only full replacement for HTTP headers.
type MCPHeadersChange struct {
	Kind  MCPSecretChangeKind
	Value map[string]string
}

// MCPEnvironmentChange is a write-only full replacement for a stdio process's
// environment.
type MCPEnvironmentChange struct {
	Kind  MCPSecretChangeKind
	Value map[string]string
}

// MCPConnectionInput is a complete connection replacement. Transport-specific
// validation happens before it becomes the domain's flat persistence descriptor.
type MCPConnectionInput struct {
	Transport     mcpserver.Transport
	URL           string
	Authorization *MCPAuthorizationChange
	Headers       *MCPHeadersChange
	Command       string
	Args          []string
	Environment   *MCPEnvironmentChange
	Dir           string
}

// MCPServerInput is a complete create/test candidate.
type MCPServerInput struct {
	Name             string
	Enabled          bool
	Description      string
	Connection       MCPConnectionInput
	Timeout          time.Duration
	DisabledTools    []string
	AutoApproveTools []string
}

// MCPServerPatch is an update command. nil preserves the current value; a
// present zero or empty collection clears it.
type MCPServerPatch struct {
	Enabled          *bool
	Description      *string
	Connection       *MCPConnectionInput
	Timeout          *time.Duration
	DisabledTools    *[]string
	AutoApproveTools *[]string
}

// Empty reports whether the update carries no mutation.
func (patch MCPServerPatch) Empty() bool {
	return patch.Enabled == nil && patch.Description == nil && patch.Connection == nil &&
		patch.Timeout == nil && patch.DisabledTools == nil && patch.AutoApproveTools == nil
}

// MCPConnection is the safe application read model for a connection. Raw
// secret-bearing values never cross the application boundary.
type MCPConnection struct {
	Transport           mcpserver.Transport
	URL                 string
	AuthorizationMasked string
	HeadersMasked       map[string]string
	Command             string
	Args                []string
	EnvironmentMasked   map[string]string
	Dir                 string
}

// MCPServer is the unified application read model: durable configuration and
// the current connection lifecycle are projected together.
type MCPServer struct {
	Name             string
	Description      string
	Connection       MCPConnection
	Timeout          time.Duration
	DisabledTools    []string
	AutoApproveTools []string
	State            MCPServerState
}

// MCPServerState is the application's complete server lifecycle. Disabled is
// represented explicitly rather than as an absent or contradictory status.
type MCPServerState struct {
	Type      MCPServerStateType
	ToolCount *int
}

type MCPServerStateType uint8

const (
	MCPServerDisabled MCPServerStateType = iota + 1
	MCPServerDisconnected
	MCPServerConnecting
	MCPServerConnected
	MCPServerFailed
	MCPServerNeedsAuth
)

// MCPServerStatus is the application status notification read model. Known is
// false after a removed server's final invalidation.
type MCPServerStatus struct {
	Name      string
	Known     bool
	State     mcpserver.ConnectionState
	ToolCount *int
}

// MCPTestResult is the semantic outcome of a non-persisting connection probe.
type MCPTestResult struct {
	OK bool
}

func mcpConnectionView(server mcpserver.Server) MCPConnection {
	return MCPConnection{
		Transport:           server.Transport,
		URL:                 server.URL,
		AuthorizationMasked: secretmask.Mask(server.Authorization),
		HeadersMasked:       maskedValues(server.Headers),
		Command:             server.Command,
		Args:                slices.Clone(server.Args),
		EnvironmentMasked:   maskedValues(server.Env),
		Dir:                 server.Dir,
	}
}

func maskedValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	masked := make(map[string]string, len(values))
	for key, value := range values {
		masked[key] = secretmask.Mask(value)
	}
	return masked
}

func mcpServerView(server mcpserver.Server, status *MCPServerStatus) MCPServer {
	view := MCPServer{
		Name:             server.Name,
		Description:      server.Description,
		Connection:       mcpConnectionView(server),
		Timeout:          server.Timeout,
		DisabledTools:    slices.Clone(server.DisabledTools),
		AutoApproveTools: slices.Clone(server.AutoApproveTools),
		State:            MCPServerState{Type: MCPServerDisconnected},
	}
	if !server.Enabled {
		view.State.Type = MCPServerDisabled
		return view
	}
	if status == nil || !status.Known {
		return view
	}
	switch status.State {
	case mcpserver.ConnectionConnecting:
		view.State.Type = MCPServerConnecting
	case mcpserver.ConnectionConnected:
		view.State.Type = MCPServerConnected
		view.State.ToolCount = status.ToolCount
	case mcpserver.ConnectionFailed:
		view.State.Type = MCPServerFailed
	case mcpserver.ConnectionNeedsAuth:
		view.State.Type = MCPServerNeedsAuth
	default:
		panic("integrations: unknown MCP connection state")
	}
	return view
}

func mcpStatusView(status mcpserver.ConnectionStatus) MCPServerStatus {
	view := MCPServerStatus{Name: status.Name, Known: true, State: status.State}
	if status.State == mcpserver.ConnectionConnected {
		count := status.ToolCount
		view.ToolCount = &count
	}
	return view
}
