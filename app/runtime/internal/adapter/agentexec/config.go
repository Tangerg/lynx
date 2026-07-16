package agentexec

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/tools"
)

// Config is the engine construction-time bundle. ChatClient is the
// only hard requirement (New errors without it); the rest are
// optional — a nil/empty field disables or defaults the corresponding
// feature, per-field docs below.
type Config struct {
	// ChatClient is the LLM client used by every action. Built from
	// a lynx model adapter (anthropic, openai, ...) at startup.
	ChatClient *chatclient.Client

	// Workdir is the DEFAULT working directory — the fallback for
	// turns that carry no session cwd. A turn that does carry one
	// (runs.start resolves Session.Cwd) overrides it everywhere
	// cwd-dependent: fs/shell tools, project skills, and the system
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

	// HistoryStore optionally supplies a persistent chat-history
	// backend (the sqlite MessageStore, redis-backed, ...). When nil the
	// engine falls back to lynx's in-process [history.InMemoryStore]
	// — fine for tests but loses history on restart.
	HistoryStore history.Store

	// Knowledge optionally supplies the LYRA.md cascade reader the agent
	// injects into every system prompt. nil disables the injection — the
	// base prompt is used verbatim. (Wire/API calls this "memory".)
	Knowledge knowledge.Store

	// Todos optionally supplies the per-session task list backing the
	// todo_write tool: when set, the tool is registered and the session's
	// current list is injected into every system prompt. nil disables the
	// feature (no tool, no injection).
	Todos todo.Store

	// Microkernel ports — injected by the composition root (runtime). Each is
	// optional; a nil port no-ops its capability (every use is nil-guarded), so
	// a bare engine still drives the loop. See port.go / doc/EXECUTION_CENTERED_ARCHITECTURE.md
	Steering  SteeringSink // turn-end steering inject (next-turn message)
	Compactor Compactor    // turn-boundary history compaction
	Extractor Extractor    // turn-boundary fact extraction → LYRA.md

	// Tool environment — assembled outside the core by the tool adapter and
	// injected by the composition root. The engine registers ToolResolver on
	// the engine, surfaces tools via tools.list, exposes MCP workspace actions
	// through narrow live-connection ports, and runs Closers at shutdown. A
	// resolver also receives the engine-built `task` delegation tool; without a
	// resolver, task is not available (the env can still pass only static tools
	// like ask_user).
	ToolResolver          toolport.ToolResolver
	Tools                 []tools.Tool                   // canonical tool list (without task)
	MCPStatusReader       toolport.MCPStatusReader       // live MCP server status read model
	MCPToolCatalog        toolport.MCPToolCatalog        // live MCP tool read model
	MCPConnectionCommands toolport.MCPConnectionCommands // reconnect / authorize configured servers
	MCPRegistryCommands   toolport.MCPRegistryCommands   // probe / configure / remove live servers
	Closers               []func() error                 // capability shutdown hooks

	// Pricing optionally computes per-round USD cost from the round's
	// provider + served model + token usage. nil leaves cost at zero (the chat
	// path gets no dollar figure from providers). Supply a rate table to surface
	// CostUSD on TurnOutput / TurnEnd. See [accounting.Pricing].
	Pricing accounting.Pricing

	// Provider is the runtime's DEFAULT provider id — the provider a turn runs
	// against when it names none (default / subtask turns). Pricing uses it to
	// attribute a default turn's cost to the right provider (a model id alone is
	// ambiguous across providers). Empty when no default is configured.
	Provider string

	// ProcessStore, when non-nil, makes the engine auto-snapshot every
	// agent process per tick to a durable backend (audit trail + the
	// foundation for resuming a paused turn across restart). nil = no
	// persistence (no per-tick disk churn). The snapshot is process-level
	// (status / blackboard / history / budget), so for a single-action
	// turn it captures the turn boundary, not mid-LLM-loop state.
	ProcessStore core.ProcessStore

	// SessionStore, when non-nil, is handed to the engine so the runtime
	// persists a sub-agent's session when it spawns one (the `task`
	// delegation). The engine doesn't touch sessions itself — it only forwards
	// this to [agent/runtime.Config] — keeping session CRUD out of the
	// turn-execution layer. nil = delegation lineage stays in-process only.
	SessionStore core.SessionStore
}
