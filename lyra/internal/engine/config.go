package engine

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
	"github.com/Tangerg/lynx/lyra/internal/infra/a2a"
	"github.com/Tangerg/lynx/lyra/internal/infra/mcp"
	"github.com/Tangerg/lynx/lyra/internal/service/codeintel"
	"github.com/Tangerg/lynx/lyra/internal/service/knowledge"
	"github.com/Tangerg/lynx/lyra/internal/service/maintenance"
)

// Config is the engine construction-time bundle. ChatClient is the
// only hard requirement (New errors without it); the rest are
// optional — a nil/empty field disables or defaults the corresponding
// feature, per-field docs below.
type Config struct {
	// ChatClient is the LLM client used by every action. Built from
	// a lynx model adapter (anthropic, openai, ...) at startup.
	ChatClient *chat.Client

	// Workdir is the DEFAULT working directory — the fallback for
	// turns that carry no session cwd. A turn that does carry one
	// (runs.start resolves Session.Cwd) overrides it everywhere
	// cwd-dependent: fs/bash tools, project skills, and the system
	// prompt's project LYRA.md + AGENTS.md cascade (see turnCwd).
	// Empty disables tool path scoping (LocalExecutor permits any
	// path) — fine for tests, not recommended for production.
	Workdir string

	// SkillsGlobalDir is the user-scope Agent Skills directory (typically
	// ~/.lyra/skills). It is merged UNDER each session's project skills
	// (<workdir>/.lyra/skills), so a project skill overrides a global one of
	// the same name. Empty disables the global layer; the skill tool still
	// appears when a project directory exists, and is absent when neither does.
	SkillsGlobalDir string

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
	MemoryService knowledge.Service

	// Compaction tunes the auto-compaction heuristic. Zero values
	// fall back to defaults (see [maintenance.CompactionConfig]).
	// Setting MaxMessages negative disables auto-compaction entirely.
	Compaction maintenance.CompactionConfig

	// MCPServers lists external MCP servers to dial at engine
	// construction. Their tools are merged into the built-in coding
	// tool set, prefixed with the server's Name so collisions across
	// servers stay separable. Empty disables MCP integration.
	MCPServers []mcp.ServerConfig

	// A2AAgents lists remote A2A (Agent-to-Agent) agents to dial at engine
	// construction. Each becomes one delegation tool in the coding set (named
	// by its config, else the resolved AgentCard), letting the chat loop hand
	// work to a remote agent. Empty disables A2A integration.
	A2AAgents []a2a.ClientConfig

	// LSPServers overrides the language-server table the code-intelligence
	// tools drive. Empty uses codeintel.DefaultServers() (gopls + typescript).
	// When set, it REPLACES the defaults wholesale (list every language you
	// want), so an operator can add servers (pyright, rust-analyzer, …) or pin
	// commands via config without a rebuild.
	LSPServers []codeintel.ServerSpec

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

	// SessionStore, when non-nil, is handed to the platform so the runtime
	// persists a sub-agent's session when it spawns one (the `task`
	// delegation). The engine doesn't touch sessions itself — it only forwards
	// this to [agent/runtime.PlatformConfig] — keeping session CRUD out of the
	// chat-execution layer. nil = delegation lineage stays in-process only.
	SessionStore core.SessionStore

	// ParkStore persists interrupted tool rounds for HITL resume.
	// When nil the engine intercepts [chat.FinishReasonInterrupt] chunks
	// itself and parks the resumable tail on the process blackboard.
	ParkStore tool.ParkStore
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
