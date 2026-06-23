// Package config loads Lyra's runtime configuration via viper.
//
// Sources, later overrides earlier:
//
//  1. Built-in defaults
//  2. config/config.yaml (or $HOME/.lyra/config.yaml) — viper
//  3. Environment variables (LYRA_* + provider {NAME}_API_KEY)
//  4. CLI flags (resolved by cmd/lyra; e.g. serve --listen)
//
// The yaml file is where the API key lives in dev; it is gitignored.
// Copy config/config.example.yaml → config/config.yaml and fill it in.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/Tangerg/lynx/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/lsp"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/mcp"
)

// ServerConfig holds the `lyra serve` HTTP transport settings. CLI
// flags override these (serve resolves flag-vs-config per field).
type ServerConfig struct {
	Listen         string
	NoLocalToken   bool
	LocalTokenPath string
	CORSOrigins    []string // empty → serve falls back to the built-in dev allowlist

	// A2AListen is the bind address for the A2A (Agent-to-Agent) endpoint
	// that exposes this Lyra agent to other agents. Empty disables it —
	// A2A serving is opt-in because it hands a remote caller the full
	// coding agent (filesystem + shell tools). Separate listener: the A2A
	// protocol is distinct from the Lyra Runtime Protocol on Listen.
	A2AListen string
}

// Config is the loaded runtime configuration.
type Config struct {
	Provider llm.Provider
	Model    string
	APIKey   string

	// BaseURL optionally overrides the provider's default API endpoint —
	// every adapter accepts one (native openai/anthropic via a request
	// option, the OpenAI-compatible delegators via their BaseURL field).
	// Empty uses the adapter's built-in default. Useful for proxies,
	// gateways, regional endpoints, and self-hosted OpenAI-compatible servers.
	BaseURL string

	// UtilityModel optionally names a cheaper / faster model for the
	// turn-boundary maintenance work — compaction summaries, fact extraction,
	// title generation — on the SAME provider (key + BaseURL) as Model. Empty
	// runs that work on the main Model. The point: a session can code with a
	// strong model (e.g. an Opus-class Model) while its background
	// summarize/extract/title calls use an inexpensive one, since those don't
	// need the headline model's quality.
	UtilityModel string

	// Online optionally enables provider-backed tools.
	Online kernel.OnlineConfig

	// MCPServers is the parsed list of external MCP servers dialed at
	// startup. First cut: sourced from LYRA_MCP_SERVERS env (yaml
	// support is a later addition).
	MCPServers []mcp.ServerConfig

	// A2AAgents is the parsed list of remote A2A agents dialed at startup.
	// Sourced from LYRA_A2A_AGENTS env (same name=value shape as
	// LYRA_MCP_SERVERS; yaml support is a later addition).
	A2AAgents []a2a.ClientConfig

	// LSPServers is the optional language-server table from yaml `lsp.servers`.
	// Empty leaves the engine on its built-in defaults (gopls + typescript);
	// when set it replaces them wholesale.
	LSPServers []lsp.ServerSpec

	// Server holds the HTTP serve settings.
	Server ServerConfig
}

// Load resolves configuration from yaml + env + defaults. A missing
// config file is fine (defaults + env only). The returned config is
// ready to hand to engine + transport wiring.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("config")      // ./config/config.yaml (run from the lyra dir)
	v.AddConfigPath("$HOME/.lyra") // ~/.lyra/config.yaml

	// No default provider — it must be set explicitly in config/config.yaml
	// or via LYRA_PROVIDER. (No vendor is privileged as the implicit default.)
	v.SetDefault("server.listen", "127.0.0.1:17171")
	v.SetDefault("server.noLocalToken", false)

	// LYRA_* env override yaml (e.g. LYRA_PROVIDER, LYRA_SERVER_LISTEN).
	v.SetEnvPrefix("LYRA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return Config{}, fmt.Errorf("config: read config file: %w", err)
		}
		// No config file — defaults + env only.
	}

	provider := llm.Provider(v.GetString("provider"))
	if provider == "" {
		return Config{}, errors.New("config: provider is required — set `provider:` in config/config.yaml or LYRA_PROVIDER (see providers.list for the supported set)")
	}
	if !llm.IsSupported(provider) {
		return Config{}, fmt.Errorf("config: unknown provider %q (see providers.list for the supported set)", provider)
	}

	model := v.GetString("model")
	if model == "" {
		model = llm.DefaultModel(provider)
	}

	// API key: yaml `apiKey`, overridden by the provider's native env var
	// (e.g. ANTHROPIC_API_KEY / OPENAI_API_KEY — see llm.APIKeyEnv) so the
	// env workflow still works.
	apiKey := v.GetString("apiKey")
	apiKeyEnv := llm.APIKeyEnv(provider)
	if envKey := os.Getenv(apiKeyEnv); envKey != "" {
		apiKey = envKey
	}
	if apiKey == "" {
		return Config{}, errors.New("config: apiKey is empty — set it in config/config.yaml or " + apiKeyEnv)
	}

	servers, err := parseMCPServers(os.Getenv("LYRA_MCP_SERVERS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: LYRA_MCP_SERVERS: %w", err)
	}

	a2aAgents, err := parseA2AAgents(os.Getenv("LYRA_A2A_AGENTS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: LYRA_A2A_AGENTS: %w", err)
	}

	lspServers, err := loadLSPServers(v)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Provider:         provider,
		Model:            model,
		APIKey:           apiKey,
		BaseURL:          v.GetString("baseURL"),
		UtilityModel: v.GetString("utilityModel"),
		Online:           loadOnline(v),
		MCPServers:       servers,
		A2AAgents:        a2aAgents,
		LSPServers:       lspServers,
		Server: ServerConfig{
			Listen:         v.GetString("server.listen"),
			NoLocalToken:   v.GetBool("server.noLocalToken"),
			LocalTokenPath: v.GetString("server.localTokenPath"),
			CORSOrigins:    v.GetStringSlice("server.corsOrigins"),
			A2AListen:      v.GetString("server.a2aListen"),
		},
	}, nil
}

// loadOnline reads the optional provider-tool credentials. yaml under
// `online:`; the LYRA_* env vars take precedence over yaml, matching
// the overall source ordering (env over file).
func loadOnline(v *viper.Viper) kernel.OnlineConfig {
	jina := cmp.Or(os.Getenv("LYRA_JINA_API_KEY"), v.GetString("online.jinaApiKey"))
	tavily := cmp.Or(os.Getenv("LYRA_TAVILY_API_KEY"), v.GetString("online.tavilyApiKey"))
	hosts := v.GetStringSlice("online.httpAllowedHosts")
	if env := os.Getenv("LYRA_HTTP_ALLOWED_HOSTS"); env != "" {
		hosts = splitHosts(env)
	}
	return kernel.OnlineConfig{
		JinaAPIKey:       jina,
		TavilyAPIKey:     tavily,
		HTTPAllowedHosts: hosts,
	}
}

// loadLSPServers reads the optional language-server table from yaml
// `lsp.servers`. Absent → nil (the engine falls back to lsp.DefaultServers()).
// mapstructure matches keys case-insensitively, so the yaml keys are
// name/command/args/languageId/extensions/rootMarkers.
func loadLSPServers(v *viper.Viper) ([]lsp.ServerSpec, error) {
	var servers []lsp.ServerSpec
	if err := v.UnmarshalKey("lsp.servers", &servers); err != nil {
		return nil, fmt.Errorf("config: lsp.servers: %w", err)
	}
	return servers, nil
}

// parseMCPServers parses the LYRA_MCP_SERVERS env var: a comma-
// separated list of "name=value" pairs. Empty input yields nil.
//
// Two value shapes:
//
//	HTTP:  name=https://mcp.example.com/   (or http://)
//	stdio: name=stdio:command arg1 arg2    (whitespace-split argv)
func parseMCPServers(raw string) ([]mcp.ServerConfig, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]mcp.ServerConfig, 0, len(parts))
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

// parseA2AAgents parses the LYRA_A2A_AGENTS env var: a comma-separated list
// of "name=cardURL" pairs, where cardURL is the base URL the remote agent's
// AgentCard is resolved from. Empty input yields nil. The name becomes the
// delegation tool's name; the first '=' separates it from the URL, so query
// strings in the URL are preserved.
func parseA2AAgents(raw string) ([]a2a.ClientConfig, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]a2a.ClientConfig, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		if eq <= 0 || eq == len(p)-1 {
			return nil, fmt.Errorf("entry %q: expected name=cardURL", p)
		}
		name := strings.TrimSpace(p[:eq])
		url := strings.TrimSpace(p[eq+1:])
		if name == "" || url == "" {
			return nil, fmt.Errorf("entry %q: name and cardURL must be non-empty", p)
		}
		out = append(out, a2a.ClientConfig{Name: name, CardURL: url})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// parseMCPServerValue dispatches by prefix. `stdio:` is a Lyra
// convention — anything else must look like an HTTP(S) URL.
func parseMCPServerValue(name, value string) (mcp.ServerConfig, error) {
	if rest, ok := strings.CutPrefix(value, "stdio:"); ok {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return mcp.ServerConfig{}, errors.New("stdio: command is empty")
		}
		fields := strings.Fields(rest)
		return mcp.ServerConfig{
			Name:      name,
			Transport: mcp.TransportStdio,
			Command:   fields[0],
			Args:      fields[1:],
		}, nil
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return mcp.ServerConfig{}, fmt.Errorf("expected http(s):// URL or stdio: prefix, got %q", value)
	}
	return mcp.ServerConfig{
		Name:      name,
		Transport: mcp.TransportHTTP,
		Endpoint:  value,
		// Optional bearer token from a per-server env, kept out of the
		// server-list string so the secret isn't co-located with the URL.
		Authorization: mcpAuthFromEnv(name),
	}, nil
}

// mcpAuthFromEnv reads an optional bearer token for HTTP MCP server `name`
// from LYRA_MCP_<NAME>_TOKEN (name upper-cased, non-alphanumerics → '_') and
// returns it as an "Authorization: Bearer <token>" header value, or "" when
// unset. It authenticates Lyra to an access-controlled MCP server. The token
// lives in its own env var (not the LYRA_MCP_SERVERS list) so the secret stays
// separate from the connection descriptor.
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

// splitHosts parses the comma-separated LYRA_HTTP_ALLOWED_HOSTS value.
func splitHosts(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
