package config

// Server holds the runtime HTTP transport settings.
type Server struct {
	Listen         string
	NoLocalToken   bool
	LocalTokenPath string
	CORSOrigins    []string // empty → server falls back to the built-in dev allowlist
}

// Online holds credentials for optional network-reaching tools. Empty
// fields leave the corresponding tool disabled.
type Online struct {
	JinaAPIKey       string
	TavilyAPIKey     string
	HTTPAllowedHosts []string
}

// MCPTransport identifies the transport selected by one parsed MCP server
// entry. Consumers explicitly map this configuration vocabulary into their
// own connection vocabulary.
type MCPTransport string

const (
	MCPTransportStdio          MCPTransport = "stdio"
	MCPTransportStreamableHTTP MCPTransport = "streamableHttp"
)

// Valid reports whether m names one supported configuration transport.
func (m MCPTransport) Valid() bool {
	return m == MCPTransportStdio || m == MCPTransportStreamableHTTP
}

// MCPServer is one MCP server entry parsed from LYRA_MCP_SERVERS. It is
// the config package's source DTO; runtime maps it into its registry model.
type MCPServer struct {
	Name          string
	Transport     MCPTransport
	Endpoint      string
	Command       string
	Args          []string
	Authorization string
}

// LSPServer is one optional language-server table entry loaded from yaml.
// Empty LSPServers means the runtime falls back to its built-in table.
type LSPServer struct {
	Name        string
	Command     string
	Args        []string
	LanguageID  string
	Extensions  []string
	RootMarkers []string
}

// A2AAgent is one remote Agent-to-Agent endpoint loaded from config.
type A2AAgent struct {
	Name              string
	CardURL           string
	AllowedRPCOrigins []string
}

// Settings is the loaded runtime configuration.
type Settings struct {
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
	// Run-boundary maintenance work — compaction summaries, fact extraction,
	// title generation — on the SAME provider (key + BaseURL) as Model. Empty
	// runs that work on the main Model. The point: a session can code with a
	// strong model (e.g. an Opus-class Model) while its background
	// summarize/extract/title calls use an inexpensive one, since those don't
	// need the headline model's quality.
	UtilityModel string

	// Online optionally enables provider-backed tools.
	Online Online

	// MCPServers is the parsed list of external MCP servers dialed at startup.
	// Sourced from LYRA_MCP_SERVERS.
	MCPServers []MCPServer

	// A2AAgents is the parsed list of remote A2A agents dialed at startup.
	// Sourced from LYRA_A2A_AGENTS; optional cross-origin RPC trust is supplied
	// separately by LYRA_A2A_RPC_ORIGINS.
	A2AAgents []A2AAgent

	// LSPServers is the optional language-server table from yaml `lsp.servers`.
	// Empty leaves the engine on its built-in defaults (gopls + typescript);
	// when set it replaces them wholesale.
	LSPServers []LSPServer

	// ToolResultOffloadThreshold is the byte size above which a single tool
	// result is offloaded out of the conversation and replaced by a head+tail
	// placeholder the model reads back via read_tool_result. Defaults to
	// [DefaultToolResultOffloadThreshold] (enabled); set `toolResultOffload.threshold: 0`
	// (or any non-positive value) in config.yaml / LYRA_TOOLRESULTOFFLOAD_THRESHOLD
	// to disable eviction.
	ToolResultOffloadThreshold int

	// SandboxShell opts the shell tool family into per-command OS isolation:
	// each command runs in an in-place jail rooted at its cwd (workspace-write
	// only, network denied, $HOME hidden, env scrubbed). Off by default (a plain
	// /bin/sh -c). Sourced from `sandbox.shell` / LYRA_SANDBOX_SHELL. On a host
	// with no isolation backend (only macOS Seatbelt today) enabling it refuses
	// startup — fail-closed, rather than silently running shells unconfined.
	SandboxShell bool

	// SandboxReadOnlyPaths re-opens declared toolchain roots below the hidden
	// home for reads under the sandbox (e.g. a language toolchain or dependency
	// cache under $HOME that a build needs). Sourced from `sandbox.readOnlyPaths`.
	// Ignored unless SandboxShell is set.
	SandboxReadOnlyPaths []string

	// Server holds the HTTP serve settings.
	Server Server
}

// DefaultToolResultOffloadThreshold is the default byte size above which a
// single tool result is offloaded (see [Settings.ToolResultOffloadThreshold]).
// ~50k bytes (≈ characters for ASCII tool output) is well past a normal result
// yet small enough that one giant file read or command dump stops re-inflating
// every later request.
const DefaultToolResultOffloadThreshold = 50_000
