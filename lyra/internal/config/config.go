// Package config loads Lyra's runtime configuration.
//
// Configuration sources (later overrides earlier):
//
//  1. Built-in defaults
//  2. ~/.lyra/config.toml (when present — not implemented yet)
//  3. Environment variables (LYRA_*)
//  4. CLI flags (resolved by cmd/lyra)
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// Provider enumerates the LLM provider Lyra talks to. M1 ships the
// two we have the smoothest lynx adapters for.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
)

// StorageKind selects the backend for session + memory state. "file"
// keeps the per-LYRA.md / sessions.json layout; "sqlite" puts both in
// one SQLite database at $LYRA_HOME/lyra.db.
type StorageKind string

const (
	StorageFile   StorageKind = "file"
	StorageSQLite StorageKind = "sqlite"
)

// Config is the loaded runtime configuration. Constructed by [Load];
// passed verbatim into engine + service wiring.
type Config struct {
	Provider Provider
	Model    string
	APIKey   string

	// Online optionally enables provider-backed tools (web fetch,
	// web search, HTTP requests). Each field is independent — set
	// only the ones you have credentials for. See [OnlineConfig].
	Online OnlineConfig

	// MCPServers is the parsed list of external MCP servers the
	// engine dials at startup. Tools from each server merge into
	// the engine's tool set under the server's Name as prefix.
	// Empty disables MCP integration. Stored directly in the
	// engine's wire format so no bridge layer is needed.
	MCPServers []engine.MCPServer

	// Storage selects the persistence backend for session +
	// memory services. Defaults to StorageFile — set LYRA_STORAGE
	// to "sqlite" to use the SQLite backend instead.
	Storage StorageKind
}

// OnlineConfig groups the credentials needed by network-reaching
// tools. Empty fields disable the corresponding tool — no tool is
// registered without explicit opt-in, so an offline-only install
// has no surprise outbound traffic.
type OnlineConfig struct {
	// JinaAPIKey enables the webfetch tool backed by Jina Reader.
	// Get a key from https://jina.ai/reader.
	JinaAPIKey string

	// TavilyAPIKey enables the websearch tool backed by Tavily.
	// Get a key from https://app.tavily.com.
	TavilyAPIKey string

	// HTTPAllowedHosts enables the httpreq tool. Pass an explicit
	// allowlist (e.g. ["api.github.com", "*.openai.com"]) — empty
	// keeps the tool disabled. Required so the LLM can't reach
	// arbitrary internal endpoints.
	HTTPAllowedHosts []string
}

// Load resolves the configuration from defaults + environment. The
// returned config is ready to hand to engine.New.
//
// M1 lookups (in priority order):
//
//   - LYRA_PROVIDER (default "anthropic")
//   - LYRA_MODEL    (default per provider)
//   - {PROVIDER}_API_KEY  (anthropic → ANTHROPIC_API_KEY etc.)
func Load() (Config, error) {
	provider := Provider(envOr("LYRA_PROVIDER", string(ProviderAnthropic)))

	switch provider {
	case ProviderAnthropic, ProviderOpenAI:
	default:
		return Config{}, errors.New("config: unknown LYRA_PROVIDER (want anthropic|openai)")
	}

	model := envOr("LYRA_MODEL", defaultModelFor(provider))
	apiKey := os.Getenv(apiKeyEnvFor(provider))
	if apiKey == "" {
		return Config{}, errors.New("config: " + apiKeyEnvFor(provider) + " is empty")
	}

	servers, err := parseMCPServers(os.Getenv("LYRA_MCP_SERVERS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: LYRA_MCP_SERVERS: %w", err)
	}

	storage := StorageKind(envOr("LYRA_STORAGE", string(StorageFile)))
	switch storage {
	case StorageFile, StorageSQLite:
	default:
		return Config{}, errors.New("config: unknown LYRA_STORAGE (want file|sqlite)")
	}

	return Config{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
		Online: OnlineConfig{
			JinaAPIKey:       os.Getenv("LYRA_JINA_API_KEY"),
			TavilyAPIKey:     os.Getenv("LYRA_TAVILY_API_KEY"),
			HTTPAllowedHosts: splitHosts(os.Getenv("LYRA_HTTP_ALLOWED_HOSTS")),
		},
		MCPServers: servers,
		Storage:    storage,
	}, nil
}

// parseMCPServers parses the LYRA_MCP_SERVERS env var: a comma-
// separated list of "name=value" pairs. Empty input yields nil
// with no error; per-entry errors include the offending fragment
// so the operator can spot the typo immediately.
//
// Two value shapes:
//
//	HTTP:  name=https://mcp.example.com/   (or http://)
//	stdio: name=stdio:command arg1 arg2    (whitespace-split argv,
//	                                        no shell interpolation)
//
// Examples:
//
//	LYRA_MCP_SERVERS="github=https://mcp.github.com/,\
//	  fs=stdio:npx -y @modelcontextprotocol/server-filesystem /workspace,\
//	  time=stdio:uvx mcp-server-time"
func parseMCPServers(raw string) ([]engine.MCPServer, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]engine.MCPServer, 0, len(parts))
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

// parseMCPServerValue dispatches by prefix. `stdio:` is a Lyra
// convention — anything else must look like an HTTP(S) URL (we
// only sanity-check the scheme so the typo "stido:" stops here
// rather than turning into a stalled HTTP dial).
func parseMCPServerValue(name, value string) (engine.MCPServer, error) {
	if rest, ok := strings.CutPrefix(value, "stdio:"); ok {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return engine.MCPServer{}, fmt.Errorf("stdio: command is empty")
		}
		fields := strings.Fields(rest)
		return engine.MCPServer{
			Name:      name,
			Transport: engine.MCPTransportStdio,
			Command:   fields[0],
			Args:      fields[1:],
		}, nil
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return engine.MCPServer{}, fmt.Errorf("expected http(s):// URL or stdio: prefix, got %q", value)
	}
	return engine.MCPServer{
		Name:      name,
		Transport: engine.MCPTransportHTTP,
		Endpoint:  value,
	}, nil
}

// splitHosts parses the comma-separated LYRA_HTTP_ALLOWED_HOSTS
// value. Empty entries are dropped so trailing commas are tolerated.
// Empty input → nil slice (tool stays disabled).
func splitHosts(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultModelFor(p Provider) string {
	switch p {
	case ProviderAnthropic:
		return "claude-3-5-haiku-20241022"
	case ProviderOpenAI:
		return "gpt-4o-mini"
	}
	return ""
}

func apiKeyEnvFor(p Provider) string {
	switch p {
	case ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	case ProviderOpenAI:
		return "OPENAI_API_KEY"
	}
	return ""
}
