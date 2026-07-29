package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
)

// KnowledgeReader is the prompt assembler's read-only view of human-authored
// LYRA.md. Writing and listing knowledge belong to the workspace use case.
type KnowledgeReader interface {
	Get(ctx context.Context, scope knowledge.Scope, dir string) (string, error)
}

// TodoReader is the execution adapter's read-only view of session todos: the
// items for prompt assembly, and the whole projection — revision included — for
// the state.snapshot the turn publishes after a replacement.
// Replacing a list and lifecycle cleanup belong to their direct consumers.
type TodoReader interface {
	List(ctx context.Context, sessionID string) ([]todo.Item, error)
	State(ctx context.Context, sessionID string) (todo.State, error)
}

// AgentMemoryReader is the prompt assembler's view of agent-maintained memory
// items — the always-on (pinned) core injected into the system prompt.
type AgentMemoryReader interface {
	Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error)
}

// MemorySearcher retrieves the memory items most relevant to a turn's message,
// for the per-turn "relevant memories" recall block (the non-pinned corpus).
type MemorySearcher interface {
	Search(ctx context.Context, scope agentmemory.Scope, project, query string, topK int) ([]agentmemory.Item, error)
}

// ProcessStore is the execution adapter's durable checkpoint boundary.
// Storage atomically owns the process tree, application usage projection,
// build compatibility metadata, replacement, and deletion semantics; the
// Agent runtime knows only how to capture and rebuild the supplied tree value.
type ProcessStore interface {
	SaveTree(ctx context.Context, tree core.ProcessSnapshotTree, checkpoint execution.ProcessCheckpoint) error
	LoadTree(ctx context.Context, rootID string) (core.ProcessSnapshotTree, execution.ProcessCheckpoint, error)
	DeleteTrees(ctx context.Context, rootIDs []string) error
}

// Config is the engine construction-time bundle. ChatClient is the
// only hard requirement (New errors without it); the rest are
// optional — a nil/empty field disables or defaults the corresponding
// feature, per-field docs below.
type Config struct {
	// BuildID is the SHA-256 identity of the running host executable. Process
	// checkpoints carry it as application metadata so a continuation cannot be
	// restored against different executable behavior.
	BuildID string

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
	Knowledge KnowledgeReader

	// AgentMemory optionally supplies the agent-maintained memory items. The
	// pinned ones are injected as the always-on core after user preferences and
	// before project LYRA.md, so explicit project instructions remain
	// authoritative. nil disables the layer.
	AgentMemory AgentMemoryReader

	// MemorySearch optionally retrieves the memory items most relevant to each
	// turn's message, injected as a separate "relevant memories" recall block. nil
	// disables per-turn recall (only the pinned core is injected).
	MemorySearch MemorySearcher

	// Todos optionally supplies the per-session task list backing the
	// todo_write tool: when set, the tool is registered and the session's
	// current list is injected into every system prompt. nil disables the
	// feature (no tool, no injection).
	Todos TodoReader

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

	// ProcessStore persists complete process trees at turn-segment boundaries.
	// nil keeps execution in memory. The application adapter, rather than the
	// Agent framework, owns durable commit and failure policy.
	ProcessStore ProcessStore

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
