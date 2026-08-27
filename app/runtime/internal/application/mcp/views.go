package mcp

import (
	"slices"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/secrets"
	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

// SecretChangeKind is the application's exact secret-mutation vocabulary.
// Persistence never has to infer intent from an empty string.
type SecretChangeKind string

const (
	SecretSet   SecretChangeKind = "set"
	SecretClear SecretChangeKind = "clear"
)

// Valid reports whether kind names one exact secret mutation.
func (s SecretChangeKind) Valid() bool { return s == SecretSet || s == SecretClear }

// String returns the stable secret-mutation name.
func (s SecretChangeKind) String() string {
	if !s.Valid() {
		return "unknown"
	}
	return string(s)
}

// AuthorizationChange is a write-only bearer-token mutation.
type AuthorizationChange struct {
	Kind  SecretChangeKind
	Value string
}

// HeadersChange is a write-only full replacement for HTTP headers.
type HeadersChange struct {
	Kind  SecretChangeKind
	Value map[string]string
}

// EnvironmentChange is a write-only full replacement for a stdio process's
// environment.
type EnvironmentChange struct {
	Kind  SecretChangeKind
	Value map[string]string
}

// ConnectionInput is a complete connection replacement. Transport-specific
// validation happens before it becomes the domain's flat persistence descriptor.
type ConnectionInput struct {
	Transport     mcpserver.Transport
	URL           string
	Authorization *AuthorizationChange
	Headers       *HeadersChange
	Command       string
	Args          []string
	Environment   *EnvironmentChange
	Dir           string
}

// ServerInput is a complete create/test candidate.
type ServerInput struct {
	Name             string
	Enabled          bool
	Description      string
	Connection       ConnectionInput
	Timeout          time.Duration
	DisabledTools    []string
	AutoApproveTools []string
}

// ServerPatch is an update command. nil preserves the current value; a
// present zero or empty collection clears it.
type ServerPatch struct {
	Enabled          *bool
	Description      *string
	Connection       *ConnectionInput
	Timeout          *time.Duration
	DisabledTools    *[]string
	AutoApproveTools *[]string
}

// Empty reports whether the update carries no mutation.
func (s ServerPatch) Empty() bool {
	return s.Enabled == nil && s.Description == nil && s.Connection == nil &&
		s.Timeout == nil && s.DisabledTools == nil && s.AutoApproveTools == nil
}

// Connection is the safe application read model for a connection. Raw
// secret-bearing values never cross the application boundary.
type Connection struct {
	Transport           mcpserver.Transport
	URL                 string
	AuthorizationMasked string
	HeadersMasked       map[string]string
	Command             string
	Args                []string
	EnvironmentMasked   map[string]string
	Dir                 string
}

// Server is the unified application read model: durable configuration and
// the current connection lifecycle are projected together.
type Server struct {
	Name             string
	Description      string
	Connection       Connection
	Timeout          time.Duration
	DisabledTools    []string
	AutoApproveTools []string
	State            ServerState
}

// ServerState is the application's complete server lifecycle. Disabled is
// represented explicitly rather than as an absent or contradictory status.
type ServerState struct {
	Type      ServerStateType
	ToolCount *int
}

type ServerStateType string

const (
	ServerDisabled     ServerStateType = "disabled"
	ServerDisconnected ServerStateType = "disconnected"
	ServerConnecting   ServerStateType = "connecting"
	ServerConnected    ServerStateType = "connected"
	ServerFailed       ServerStateType = "failed"
	ServerNeedsAuth    ServerStateType = "needsAuth"
)

// Valid reports whether s belongs to the complete MCP server lifecycle.
func (s ServerStateType) Valid() bool {
	switch s {
	case ServerDisabled, ServerDisconnected, ServerConnecting, ServerConnected,
		ServerFailed, ServerNeedsAuth:
		return true
	default:
		return false
	}
}

// String returns the stable server-state name.
func (s ServerStateType) String() string {
	if !s.Valid() {
		return "unknown"
	}
	return string(s)
}

// ServerStatus is the application status notification read model. Known is
// false after a removed server's final invalidation.
type ServerStatus struct {
	Name      string
	Known     bool
	State     mcpserver.ConnectionState
	ToolCount *int
}

// TestResult is the semantic outcome of a non-persisting connection probe.
type TestResult struct {
	OK bool
}

func connectionView(server mcpserver.Server) Connection {
	return Connection{
		Transport:           server.Transport,
		URL:                 server.URL,
		AuthorizationMasked: secrets.Mask(server.Authorization),
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
		masked[key] = secrets.Mask(value)
	}
	return masked
}

func serverView(server mcpserver.Server, status *ServerStatus) Server {
	view := Server{
		Name:             server.Name,
		Description:      server.Description,
		Connection:       connectionView(server),
		Timeout:          server.Timeout,
		DisabledTools:    slices.Clone(server.DisabledTools),
		AutoApproveTools: slices.Clone(server.AutoApproveTools),
		State:            ServerState{Type: ServerDisconnected},
	}
	if !server.Enabled {
		view.State.Type = ServerDisabled
		return view
	}
	if status == nil || !status.Known {
		return view
	}
	switch status.State {
	case mcpserver.ConnectionConnecting:
		view.State.Type = ServerConnecting
	case mcpserver.ConnectionConnected:
		view.State.Type = ServerConnected
		view.State.ToolCount = status.ToolCount
	case mcpserver.ConnectionFailed:
		view.State.Type = ServerFailed
	case mcpserver.ConnectionNeedsAuth:
		view.State.Type = ServerNeedsAuth
	default:
		panic("mcp: unknown MCP connection state")
	}
	return view
}

func statusView(status mcpserver.ConnectionStatus) ServerStatus {
	view := ServerStatus{Name: status.Name, Known: true, State: status.State}
	if status.State == mcpserver.ConnectionConnected {
		count := status.ToolCount
		view.ToolCount = &count
	}
	return view
}
