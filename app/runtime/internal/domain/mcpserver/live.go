package mcpserver

import (
	"errors"
	"maps"
	"slices"
	"time"
)

// The live-connection vocabulary: the value types that describe an MCP server's
// process-local connection (§9 — the live projection of the persisted registry,
// as opposed to the durable [Server] entries this package also owns). The
// capability layer and delivery share these DTOs; the concrete MCP adapter maps
// them to its own dial/session types at the infra boundary.

// LiveConfig is the live MCP server descriptor the capability layer hands to the
// MCP connection adapter to open a connection. [ConfigFromServer] projects a
// persisted registry [Server] into it; the concrete MCP adapter maps it to its
// own dial config at the infra boundary.
type LiveConfig struct {
	Name          string
	Transport     Transport
	Endpoint      string
	Command       string
	Args          []string
	Env           []string
	Dir           string
	Authorization string
	Headers       map[string]string
	Timeout       time.Duration
}

// ToolInfo is one tool advertised by a connected MCP server.
type ToolInfo struct {
	Server      string
	Name        string
	Description string
	InputSchema InputSchema
}

// ConnectionState is the lifecycle state of a configured MCP connection.
// Keeping this vocabulary in the domain prevents infrastructure and delivery
// adapters from inventing subtly different string values.
type ConnectionState string

const (
	ConnectionConnecting ConnectionState = "connecting"
	ConnectionConnected  ConnectionState = "connected"
	ConnectionFailed     ConnectionState = "failed"
	ConnectionNeedsAuth  ConnectionState = "needsAuth"
)

// ConnectionStatus is the per-server live connection state exposed by the MCP
// control plane: connected or boot-failed alike. Err carries the most recent
// connection failure for ConnectionFailed and ConnectionNeedsAuth.
type ConnectionStatus struct {
	Name  string
	State ConnectionState
	Err   error
}

// ErrUnknownServer is returned when a live MCP operation addresses a server that
// was never configured.
var ErrUnknownServer = errors.New("mcp: unknown server")

// ConfigsForEnabledServers returns the live-connection descriptors projected
// from one registry snapshot (enabled servers only).
func ConfigsForEnabledServers(servers []Server) []LiveConfig {
	var out []LiveConfig
	for _, s := range servers {
		if s.Enabled {
			out = append(out, ConfigFromServer(s))
		}
	}
	return out
}

// ConfigFromServer maps a persisted registry entry to the live MCP dial
// descriptor. Tool-level gating (DisabledTools / AutoApproveTools) is applied at
// toolset build / approval, not at connection setup, so it has no place here.
// Env is flattened from the registry's KEY→value map to the "KEY=value" slice
// the stdio adapter consumes.
func ConfigFromServer(s Server) LiveConfig {
	cfg := LiveConfig{Name: s.Name, Transport: s.Transport, Timeout: s.Timeout}
	switch s.Transport {
	case TransportStreamableHTTP:
		cfg.Endpoint = s.URL
		cfg.Authorization = s.Authorization
		cfg.Headers = maps.Clone(s.Headers)
	case TransportStdio:
		cfg.Command = s.Command
		cfg.Args = slices.Clone(s.Args)
		cfg.Env = envMapToSlice(s.SafeEnv())
		cfg.Dir = s.Dir
	}
	return cfg
}

// envMapToSlice flattens a KEY→value map to the "KEY=value" slice exec wants,
// sorted by key so the dialed env is deterministic (stable across restarts and
// in tests). nil/empty yields nil.
func envMapToSlice(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}
