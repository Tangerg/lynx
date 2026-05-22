// Package config loads Lyra's runtime configuration. M1 keeps the
// shape minimal — just enough to pick a model provider and its API
// key. Later milestones extend with permission mode, sandbox profile,
// MCP server list, etc.
//
// Configuration sources (later overrides earlier):
//
//  1. Built-in defaults
//  2. ~/.lyra/config.toml (when present)
//  3. Environment variables (LYRA_*)
//  4. CLI flags (resolved by cmd/lyra)
//
// M1 implements 1 + 3 only — TOML loading lands in M2.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Provider enumerates the LLM provider Lyra talks to. M1 ships the
// two we have the smoothest lynx adapters for.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
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
	// Empty disables MCP integration.
	MCPServers []MCPServer
}

// MCPServer is the parsed form of one entry in
// LYRA_MCP_SERVERS — a logical name + Streamable HTTP endpoint.
type MCPServer struct {
	Name     string
	Endpoint string
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
	}, nil
}

// parseMCPServers parses the LYRA_MCP_SERVERS env var: a comma-
// separated list of "name=url" pairs. Empty input yields nil with
// no error; per-entry errors include the offending fragment so
// the operator can spot the typo immediately.
//
//	LYRA_MCP_SERVERS="github=https://mcp.github.com/,lsp=https://mcp.lsp.local/"
func parseMCPServers(raw string) ([]MCPServer, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]MCPServer, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		if eq <= 0 || eq == len(p)-1 {
			return nil, fmt.Errorf("entry %q: expected name=url", p)
		}
		name := strings.TrimSpace(p[:eq])
		endpoint := strings.TrimSpace(p[eq+1:])
		if name == "" || endpoint == "" {
			return nil, fmt.Errorf("entry %q: name and url must be non-empty", p)
		}
		out = append(out, MCPServer{Name: name, Endpoint: endpoint})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
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
