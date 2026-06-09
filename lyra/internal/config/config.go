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
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/mcp"
)

// Provider enumerates the LLM provider Lyra talks to.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderMoonshot  Provider = "moonshot" // Kimi (OpenAI-compatible)
	ProviderDeepSeek  Provider = "deepseek" // DeepSeek (OpenAI-compatible)
)

// providerInfo is the per-provider metadata table — the single place
// that knows each provider's default model + the env var its key reads
// from. A provider is "known" iff it has a row here. Adding one = a row
// here + a case in BuildChatClient's model construction; the validate /
// default-model / key-env lookups all read this map instead of each
// repeating a switch.
var providerInfo = map[Provider]struct {
	defaultModel string
	apiKeyEnv    string
}{
	ProviderAnthropic: {"claude-3-5-haiku-20241022", "ANTHROPIC_API_KEY"},
	ProviderOpenAI:    {"gpt-4o-mini", "OPENAI_API_KEY"},
	ProviderMoonshot:  {"kimi-k2-0905-preview", "MOONSHOT_API_KEY"},
	ProviderDeepSeek:  {"deepseek-v4-flash", "DEEPSEEK_API_KEY"},
}

// ServerConfig holds the `lyra serve` HTTP transport settings. CLI
// flags override these (serve resolves flag-vs-config per field).
type ServerConfig struct {
	Listen         string
	NoLocalToken   bool
	LocalTokenPath string
	CORSOrigins    []string // empty → serve falls back to the built-in dev allowlist
}

// Config is the loaded runtime configuration.
type Config struct {
	Provider Provider
	Model    string
	APIKey   string

	// BaseURL optionally overrides the provider's default API endpoint —
	// every adapter accepts one (native openai/anthropic via a request
	// option, the OpenAI-compatible delegators via their BaseURL field).
	// Empty uses the adapter's built-in default. Useful for proxies,
	// gateways, regional endpoints, and self-hosted OpenAI-compatible servers.
	BaseURL string

	// Online optionally enables provider-backed tools.
	Online engine.OnlineConfig

	// MCPServers is the parsed list of external MCP servers dialed at
	// startup. First cut: sourced from LYRA_MCP_SERVERS env (yaml
	// support is a later addition).
	MCPServers []mcp.ServerConfig

	// A2AAgents is the parsed list of remote A2A agents dialed at startup.
	// Sourced from LYRA_A2A_AGENTS env (same name=value shape as
	// LYRA_MCP_SERVERS; yaml support is a later addition).
	A2AAgents []a2a.ClientConfig

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

	provider := Provider(v.GetString("provider"))
	if provider == "" {
		return Config{}, errors.New("config: provider is required — set `provider:` in config/config.yaml or LYRA_PROVIDER (anthropic|openai|moonshot|deepseek)")
	}
	info, ok := providerInfo[provider]
	if !ok {
		return Config{}, errors.New("config: unknown provider (want anthropic|openai|moonshot|deepseek)")
	}

	model := v.GetString("model")
	if model == "" {
		model = info.defaultModel
	}

	// API key: yaml `apiKey`, overridden by the provider's native env
	// var ({ANTHROPIC,OPENAI,MOONSHOT,DEEPSEEK}_API_KEY) so the old env
	// workflow still works.
	apiKey := v.GetString("apiKey")
	if envKey := os.Getenv(info.apiKeyEnv); envKey != "" {
		apiKey = envKey
	}
	if apiKey == "" {
		return Config{}, errors.New("config: apiKey is empty — set it in config/config.yaml or " + info.apiKeyEnv)
	}

	servers, err := parseMCPServers(os.Getenv("LYRA_MCP_SERVERS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: LYRA_MCP_SERVERS: %w", err)
	}

	a2aAgents, err := parseA2AAgents(os.Getenv("LYRA_A2A_AGENTS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: LYRA_A2A_AGENTS: %w", err)
	}

	return Config{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		BaseURL:    v.GetString("baseURL"),
		Online:     loadOnline(v),
		MCPServers: servers,
		A2AAgents:  a2aAgents,
		Server: ServerConfig{
			Listen:         v.GetString("server.listen"),
			NoLocalToken:   v.GetBool("server.noLocalToken"),
			LocalTokenPath: v.GetString("server.localTokenPath"),
			CORSOrigins:    v.GetStringSlice("server.corsOrigins"),
		},
	}, nil
}

// loadOnline reads the optional provider-tool credentials. yaml under
// `online:`, with the legacy LYRA_* env vars taking precedence so the
// old workflow keeps working.
func loadOnline(v *viper.Viper) engine.OnlineConfig {
	jina := cmp.Or(os.Getenv("LYRA_JINA_API_KEY"), v.GetString("online.jinaApiKey"))
	tavily := cmp.Or(os.Getenv("LYRA_TAVILY_API_KEY"), v.GetString("online.tavilyApiKey"))
	hosts := v.GetStringSlice("online.httpAllowedHosts")
	if env := os.Getenv("LYRA_HTTP_ALLOWED_HOSTS"); env != "" {
		hosts = splitHosts(env)
	}
	return engine.OnlineConfig{
		JinaAPIKey:       jina,
		TavilyAPIKey:     tavily,
		HTTPAllowedHosts: hosts,
	}
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
			return nil, fmt.Errorf("config.parseDotEnvList: entry %q: expected name=value", p)
		}
		name := strings.TrimSpace(p[:eq])
		value := strings.TrimSpace(p[eq+1:])
		if name == "" || value == "" {
			return nil, fmt.Errorf("config.parseDotEnvList: entry %q: name and value must be non-empty", p)
		}

		srv, err := parseMCPServerValue(name, value)
		if err != nil {
			return nil, fmt.Errorf("config.parseDotEnvList: entry %q: %w", p, err)
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
		return mcp.ServerConfig{}, fmt.Errorf("config.parseMCPServerValue: expected http(s):// URL or stdio: prefix, got %q", value)
	}
	return mcp.ServerConfig{
		Name:      name,
		Transport: mcp.TransportHTTP,
		Endpoint:  value,
	}, nil
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
