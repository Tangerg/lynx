// Package mcpserver models user-defined MCP server connections. It owns server
// identity, transport configuration, enablement, credential-bearing fields,
// and per-tool policy; connection lifecycle and persistence are outside this
// package.
package mcpserver

import (
	"errors"
	"fmt"
	"time"
)

// Transport names an MCP server connection mode using the standard
// `mcpServers` vocabulary. It is shared by persisted and live domain values.
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
	// the registry but contributes no tools.
	Enabled bool

	// Description is an optional human note.
	Description string

	// URL is the Streamable HTTP endpoint. Used when Transport == [TransportStreamableHTTP].
	URL string

	// Authorization, when set, is sent as the HTTP `Authorization` header
	// (typically "Bearer <token>") — HTTP transport only. It is sensitive and
	// must never be logged or exposed without masking. The dedicated value wins over any
	// "Authorization" entry in [Server.Headers].
	Authorization string

	// Headers carries extra static HTTP request headers (e.g. "X-API-Key") sent
	// on every request — HTTP transport only. Values are sensitive and must
	// never be logged or exposed without masking because arbitrary headers may carry
	// credentials.
	Headers map[string]string

	// Command is the executable to spawn. Used when Transport == [TransportStdio].
	Command string

	// Args are the command arguments (stdio).
	Args []string

	// Env REPLACES the subprocess environment (stdio) as a KEY→value map; it does
	// not extend the parent env. Values are sensitive and must never be logged or
	// exposed without masking.
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
// are blank before connection-specific state is attached.
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
		if len(s.Args) > 0 {
			return fmt.Errorf("mcpserver %q: Args apply to stdio transport only", s.Name)
		}
		if len(s.Env) > 0 {
			return fmt.Errorf("mcpserver %q: Env applies to stdio transport only", s.Name)
		}
		if s.Dir != "" {
			return fmt.Errorf("mcpserver %q: Dir applies to stdio transport only", s.Name)
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
