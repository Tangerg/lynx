package engine

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
	lyramem "github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/mcp"
)

// Config is the engine construction-time bundle. All fields are
// required — engine assumes its dependencies are wired before
// construction.
type Config struct {
	// ChatClient is the LLM client used by every action. Built from
	// a lynx model adapter (anthropic, openai, ...) at startup.
	ChatClient *chat.Client

	// Workdir is the filesystem root every Lyra-shipped tool is
	// scoped to. Empty string disables the scoping (LocalExecutor
	// permits any path) — fine for tests, not recommended for
	// production. Typical value: the user's project cwd.
	Workdir string

	// Online controls which network-reaching tools (webfetch /
	// websearch / httpreq) are registered. Each tool is independent;
	// missing credentials skip just that tool.
	Online OnlineConfig

	// MemoryStore optionally supplies a persistent chat-memory
	// backend (the sqlite MessageStore, redis-backed, ...). When nil the
	// engine falls back to lynx's in-process [memory.InMemoryStore]
	// — fine for tests but loses history on restart.
	MemoryStore memory.Store

	// MemoryService optionally supplies the LYRA.md cascade reader
	// the agent injects into every system prompt. nil disables the
	// injection — the base prompt is used verbatim.
	MemoryService lyramem.Service

	// Compaction tunes the auto-compaction heuristic. Zero values
	// fall back to defaults (see [CompactionConfig]). Setting
	// MaxMessages negative disables auto-compaction entirely.
	Compaction CompactionConfig

	// MCPServers lists external MCP servers to dial at engine
	// construction. Their tools are merged into the built-in coding
	// tool set, prefixed with the server's Name so collisions across
	// servers stay separable. Empty disables MCP integration.
	MCPServers []mcp.ServerConfig

	// Pricing optionally computes per-round USD cost from the served
	// model + token usage. nil leaves cost at zero (the chat path gets
	// no dollar figure from providers). Supply a rate table to surface
	// CostUSD on ChatOutput / TurnEnd. See [Pricing].
	Pricing Pricing

	// ProcessStore, when non-nil, makes the platform auto-snapshot every
	// agent process per tick to a durable backend (audit trail + the
	// foundation for resuming a paused turn across restart). nil = no
	// persistence (no per-tick disk churn). The snapshot is process-level
	// (status / blackboard / history / budget), so for a single-action
	// chat turn it captures the turn boundary, not mid-LLM-loop state.
	ProcessStore core.ProcessStore
}

// OnlineConfig groups the credentials network-reaching tools need
// (webfetch / websearch / httpreq). Empty fields disable the
// corresponding tool — no tool is registered without explicit
// opt-in, so an offline-only install makes no surprise outbound
// calls. This is the canonical definition; the config layer stores
// it directly (no bridge mapping), keeping engine free of any
// dependency on config.
type OnlineConfig struct {
	// JinaAPIKey enables the webfetch tool backed by Jina Reader.
	JinaAPIKey string

	// TavilyAPIKey enables the websearch tool backed by Tavily.
	TavilyAPIKey string

	// HTTPAllowedHosts enables the httpreq tool. Pass an explicit
	// allowlist (e.g. ["api.github.com", "*.openai.com"]) — empty
	// keeps the tool disabled so the LLM can't reach arbitrary
	// internal endpoints.
	HTTPAllowedHosts []string
}
