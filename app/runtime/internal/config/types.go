package config

// ServerConfig holds the runtime HTTP transport settings.
type ServerConfig struct {
	Listen         string
	NoLocalToken   bool
	LocalTokenPath string
	CORSOrigins    []string // empty → server falls back to the built-in dev allowlist
}

// OnlineConfig holds credentials for optional network-reaching tools. Empty
// fields leave the corresponding tool disabled.
type OnlineConfig struct {
	JinaAPIKey          string
	TavilyAPIKey        string
	HTTPAllowedHosts    []string
	SourcegraphEndpoint string
	SourcegraphToken    string
}

// MCP transport names emitted by the config parser.
const (
	MCPTransportStdio          = "stdio"
	MCPTransportStreamableHTTP = "streamableHttp"
)

// MCPServerConfig is one env-sourced MCP server entry. It is the config
// package's source DTO; runtime maps it into its MCP-server registry model.
type MCPServerConfig struct {
	Name          string
	Transport     string
	Endpoint      string
	Command       string
	Args          []string
	Authorization string
}

// LSPServerConfig is one optional language-server table entry loaded from yaml.
// Empty LSPServers means the runtime falls back to its built-in table.
type LSPServerConfig struct {
	Name        string
	Command     string
	Args        []string
	LanguageID  string
	Extensions  []string
	RootMarkers []string
}

// A2AAgentConfig is one remote Agent-to-Agent endpoint loaded from config.
type A2AAgentConfig struct {
	Name    string
	CardURL string
}

// Config is the loaded runtime configuration.
type Config struct {
	Provider string
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
	Online OnlineConfig

	// MCPServers is the parsed list of external MCP servers dialed at startup.
	// First cut: sourced from LYRA_MCP_SERVERS env (yaml support is a later
	// addition).
	MCPServers []MCPServerConfig

	// A2AAgents is the parsed list of remote A2A agents dialed at startup.
	// Sourced from LYRA_A2A_AGENTS env (same name=value shape as
	// LYRA_MCP_SERVERS; yaml support is a later addition).
	A2AAgents []A2AAgentConfig

	// LSPServers is the optional language-server table from yaml `lsp.servers`.
	// Empty leaves the engine on its built-in defaults (gopls + typescript);
	// when set it replaces them wholesale.
	LSPServers []LSPServerConfig

	// Server holds the HTTP serve settings.
	Server ServerConfig
}
