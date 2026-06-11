package engine

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
	"github.com/Tangerg/lynx/lyra/internal/engine/toolset"
	"github.com/Tangerg/lynx/lyra/internal/service/knowledge"
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

	// MemoryStore optionally supplies a persistent chat-memory
	// backend (the sqlite MessageStore, redis-backed, ...). When nil the
	// engine falls back to lynx's in-process [memory.InMemoryStore]
	// — fine for tests but loses history on restart.
	MemoryStore memory.Store

	// MemoryService optionally supplies the LYRA.md cascade reader
	// the agent injects into every system prompt. nil disables the
	// injection — the base prompt is used verbatim.
	MemoryService knowledge.Service

	// Microkernel ports — injected by the composition root (runtime). Each is
	// optional; a nil port no-ops its capability (every use is nil-guarded), so
	// a bare engine still drives the loop. See port.go / doc/MICROKERNEL.md.
	Conversation Conversation // LLM message-history facade (fork/rollback/steering)
	Compactor    Compactor    // turn-boundary history compaction
	Extractor    Extractor    // turn-boundary fact extraction → LYRA.md
	Planner      Planner      // plan-mode plan generation

	// Tool environment — assembled outside the core by [toolset.Build] and
	// injected by the composition root. The engine registers ToolResolver on
	// the platform, exposes MCP as its workspace.mcp.* facade, surfaces Tools
	// (plus the engine-built task/ask_user) via tools.list, and runs Closers at
	// shutdown. A nil ToolResolver yields an empty (no-tool) environment.
	ToolResolver *toolset.Resolver
	Tools        []chat.Tool        // canonical tool list (without task/ask_user)
	MCP          toolset.MCPControl // live-MCP-connections facade
	Closers      []func() error     // capability shutdown hooks

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

// OnlineConfig groups the credentials network-reaching tools need. Defined in
// the toolset layer (which builds those tools); aliased here so engine.Config
// and the config layer name one type without importing toolset everywhere.
type OnlineConfig = toolset.OnlineConfig
