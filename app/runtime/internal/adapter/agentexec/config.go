package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
)

// CuratedMemoryReader is the prompt assembler's project-scoped view of
// agent-maintained memory.
type CuratedMemoryReader interface {
	CuratedMemory(ctx context.Context, project string) (knowledge.Curated, error)
}

// Config is the engine construction-time bundle. ChatClient is the
// only hard requirement (New errors without it); the rest are
// optional — a nil/empty field disables or defaults the corresponding
// feature, per-field docs below.
type Config struct {
	// BuildID is the SHA-256 identity of the running host executable. Durable
	// runtimes require the exact "sha256:<hex>" value so process snapshots
	// cannot be restored against different executable behavior.
	BuildID string

	// SnapshotFailurePolicy is fixed by the application to fail the process.
	// A durable Runtime must never continue after losing snapshot durability.
	SnapshotFailurePolicy agentruntime.SnapshotFailurePolicy

	// ChatClient is the LLM client used by every action. Built from
	// a lynx model adapter (anthropic, openai, ...) at startup.
	ChatClient *chatclient.Client

	// Workdir is the DEFAULT working directory — the fallback for
	// turns that carry no session cwd. A turn that does carry one
	// (runs.start resolves Session.Cwd) overrides it everywhere
	// cwd-dependent: fs/shell tools, project skills, curated memory, and the
	// system prompt's project LYRA.md + AGENTS.md cascade (see turnCwd).
	// Empty disables tool path scoping (LocalExecutor permits any
	// path) — fine for tests, not recommended for production.
	Workdir string

	// HistoryStore optionally supplies a persistent chat-history
	// backend (the sqlite MessageStore, redis-backed, ...). When nil the
	// engine falls back to lynx's in-process [history.InMemoryStore]
	// — fine for tests but loses history on restart.
	HistoryStore history.Store

	// Knowledge optionally supplies the human-authored LYRA.md cascade reader.
	// nil disables that prompt layer; curated memory and discovered AGENTS.md
	// remain independent. (Wire/API calls this "memory".)
	Knowledge knowledge.Store

	// CuratedMemory optionally supplies the agent-maintained project memory
	// projection. It is read after user preferences and before project LYRA.md,
	// so explicit project instructions remain authoritative. nil disables it.
	CuratedMemory CuratedMemoryReader

	// Todos optionally supplies the per-session task list backing the
	// todo_write tool: when set, the tool is registered and the session's
	// current list is injected into every system prompt. nil disables the
	// feature (no tool, no injection).
	Todos todo.Store

	// ToolResolver supplies the execution-time role groups and accepts the task
	// delegation tool that can only be built after the subtask Agent deploys.
	// Catalogs, MCP controls, and shutdown hooks stay with toolset/bootstrap.
	ToolResolver toolport.ToolResolver

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

	// ChildSessionStore, when non-nil, is handed to the engine so the runtime
	// persists a sub-agent's session when it spawns one (the `task`
	// delegation). It is distinct from root multi-turn session persistence;
	// Lyra owns root session CRUD directly. nil keeps delegation lineage only
	// in process snapshots.
	ChildSessionStore core.SessionStore

	// ToolResultStore backs tool-result eviction: a single tool output larger
	// than ToolResultThreshold is offloaded here and replaced in history by a
	// head+tail preview the model can read back via read_tool_result. nil
	// disables eviction (results always flow to history in full).
	ToolResultStore toolResultOffloader

	// ToolResultThreshold is the byte size above which a single tool result is
	// offloaded (see ToolResultStore). Zero or negative disables eviction
	// regardless of ToolResultStore.
	ToolResultThreshold int
}
