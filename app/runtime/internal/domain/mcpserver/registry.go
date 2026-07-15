// Package mcpserver is Lyra's MCP-server registry — the runtime-mutable set
// of MCP servers Lyra dials for tools, each carrying the transport descriptor
// a connection needs plus its enablement and per-tool gating.
//
// It mirrors the provider registry ([internal/domain/provider]): a persisted,
// runtime-editable set seeded at startup (from the LYRA_MCP_SERVERS env) and
// edited at runtime via workspace.mcp.configure / remove / setEnabled. Persisted
// backends (sqlite) keep runtime edits across restarts. Unlike providers there
// is no "supported set" to seed — every entry is a user-defined server, so the
// registry is a plain create/update/delete set, not a seeded catalog.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/secret"
)

// Transport names an MCP server connection mode using the standard
// `mcpServers` vocabulary. It is shared by persisted and live domain values;
// only the infrastructure adapter maps it to SDK-specific transport values.
type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamableHttp"
)

// Server is one registry entry: an MCP server descriptor plus its enablement
// and per-tool gating. Name is the primary key and the prefix that namespaces
// the server's tools ("<name>_<tool>") across servers.
type Server struct {
	// Name identifies the server and namespaces its tools. Required, unique.
	Name string

	// Transport is [TransportStdio] or [TransportStreamableHTTP]. Required.
	Transport Transport

	// Enabled gates whether the server is dialed. A disabled server stays in
	// the registry (so the UI can list + re-enable it) but contributes no tools.
	Enabled bool

	// Description is an optional human note shown in the UI.
	Description string

	// URL is the Streamable HTTP endpoint. Used when Transport == [TransportStreamableHTTP].
	URL string

	// Authorization, when set, is sent as the HTTP `Authorization` header
	// (typically "Bearer <token>") — HTTP transport only. Stored raw, masked at
	// the wire boundary, never logged. The dedicated bearer wins over any
	// "Authorization" entry in [Server.Headers].
	Authorization string

	// Headers carries extra static HTTP request headers (e.g. "X-API-Key") sent
	// on every request — HTTP transport only. Unlike Authorization these are not
	// masked (they're treated as non-secret config, matching how the ecosystem
	// surfaces a headers map); put a bearer token in Authorization, not here.
	Headers map[string]string

	// Command is the executable to spawn. Used when Transport == [TransportStdio].
	Command string

	// Args are the command arguments (stdio).
	Args []string

	// Env REPLACES the subprocess environment (stdio) as a KEY→value map; it does
	// not extend the parent env. The dial layer flattens it to "KEY=value".
	Env map[string]string

	// Dir sets the subprocess working directory; empty inherits the parent's (stdio).
	Dir string

	// Timeout bounds the connection handshake (both transports); zero leaves it
	// unbounded beyond the caller's ctx.
	Timeout time.Duration

	// DisabledTools hides these tools from the model entirely (a blacklist —
	// every other tool the server advertises stays available, so new tools are
	// exposed by default).
	DisabledTools []string

	// AutoApproveTools lists tools whose calls skip the HITL approval gate (a
	// whitelist — MCP tools otherwise follow normal approval, since a remote
	// server's tools are arbitrary capability that shouldn't auto-run by default).
	AutoApproveTools []string
}

// Validate reports whether the server is well-formed for its transport: the
// chosen transport's required field is set and the other transport's fields
// are blank at the registry boundary, before runtime-specific dial state is
// attached.
func (s Server) Validate() error {
	if s.Name == "" {
		return errors.New("mcpserver: Name is required")
	}
	if s.Timeout < 0 {
		return fmt.Errorf("mcpserver %q: Timeout must be non-negative", s.Name)
	}
	switch s.Transport {
	case TransportStreamableHTTP:
		if s.URL == "" {
			return fmt.Errorf("mcpserver %q: URL is required for streamableHttp transport", s.Name)
		}
		if s.Command != "" {
			return fmt.Errorf("mcpserver %q: Command must be empty for streamableHttp transport", s.Name)
		}
	case TransportStdio:
		if s.Command == "" {
			return fmt.Errorf("mcpserver %q: Command is required for stdio transport", s.Name)
		}
		if s.URL != "" {
			return fmt.Errorf("mcpserver %q: URL must be empty for stdio transport", s.Name)
		}
		if s.Authorization != "" {
			return fmt.Errorf("mcpserver %q: Authorization applies to http transport only", s.Name)
		}
		if len(s.Headers) > 0 {
			return fmt.Errorf("mcpserver %q: Headers apply to http transport only", s.Name)
		}
	default:
		return fmt.Errorf("mcpserver %q: unknown transport %q (want %q or %q)", s.Name, s.Transport, TransportStdio, TransportStreamableHTTP)
	}
	return nil
}

// MaskedAuthorization renders the bearer token for the wire: "" when unset,
// otherwise the runtime's standard redacted form.
func (s Server) MaskedAuthorization() string {
	return secret.Mask(s.Authorization)
}

// Registry is the MCP-server registry. All methods are safe for concurrent use.
type Registry interface {
	// List returns every registered server, enabled or not, sorted by Name.
	List(ctx context.Context) ([]Server, error)

	// Get returns one server by name; ok is false when unknown.
	Get(ctx context.Context, name string) (Server, bool, error)

	// Configure upserts a server by Name, persisting the change. Used both to
	// seed at startup and to apply a runtime workspace.mcp.configure.
	Configure(ctx context.Context, s Server) error

	// Remove deletes a server by name. Removing an unknown name is a no-op.
	Remove(ctx context.Context, name string) error

	// SetEnabled flips a server's enablement by name, persisting the change.
	SetEnabled(ctx context.Context, name string, enabled bool) error
}
