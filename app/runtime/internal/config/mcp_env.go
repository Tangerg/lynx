package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// parseMCPServers parses the LYRA_MCP_SERVERS env var: a comma-separated list
// of "name=value" pairs. Empty input yields nil.
//
// Two value shapes:
//
//	HTTP:  name=https://mcp.example.com/   (or http://)
//	stdio: name=stdio:command arg1 arg2    (whitespace-split argv)
func parseMCPServers(raw string) ([]MCPServerConfig, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]MCPServerConfig, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		if eq <= 0 || eq == len(p)-1 {
			return nil, fmt.Errorf("entry %q: expected name=value", p)
		}
		name := strings.TrimSpace(p[:eq])
		value := strings.TrimSpace(p[eq+1:])
		if name == "" || value == "" {
			return nil, fmt.Errorf("entry %q: name and value must be non-empty", p)
		}

		srv, err := parseMCPServerValue(name, value)
		if err != nil {
			return nil, fmt.Errorf("entry %q: %w", p, err)
		}
		out = append(out, srv)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// parseMCPServerValue dispatches by prefix. `stdio:` is a Lyra convention —
// anything else must look like an HTTP(S) URL.
func parseMCPServerValue(name, value string) (MCPServerConfig, error) {
	if rest, ok := strings.CutPrefix(value, "stdio:"); ok {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return MCPServerConfig{}, errors.New("stdio: command is empty")
		}
		fields := strings.Fields(rest)
		return MCPServerConfig{
			Name:      name,
			Transport: MCPTransportStdio,
			Command:   fields[0],
			Args:      fields[1:],
		}, nil
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return MCPServerConfig{}, fmt.Errorf("expected http(s):// URL or stdio: prefix, got %q", value)
	}
	return MCPServerConfig{
		Name:      name,
		Transport: MCPTransportStreamableHTTP,
		Endpoint:  value,
		// Optional bearer token from a per-server env, kept out of the
		// server-list string so the secret isn't co-located with the URL.
		Authorization: mcpAuthFromEnv(name),
	}, nil
}

// mcpAuthFromEnv reads an optional bearer token for HTTP MCP server `name` from
// LYRA_MCP_<NAME>_TOKEN (name upper-cased, non-alphanumerics → '_') and returns
// it as an "Authorization: Bearer <token>" header value, or "" when unset. It
// authenticates Lyra to an access-controlled MCP server. The token lives in its
// own env var (not the LYRA_MCP_SERVERS list) so the secret stays separate from
// the connection descriptor.
func mcpAuthFromEnv(name string) string {
	if tok := os.Getenv("LYRA_MCP_" + envTokenKey(name) + "_TOKEN"); tok != "" {
		return "Bearer " + tok
	}
	return ""
}

// envTokenKey normalizes a server name into an env-var-safe fragment:
// upper-cased, every non-alphanumeric rune replaced by '_'.
func envTokenKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
