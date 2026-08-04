package agentexec

import (
	"context"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
)

// KnowledgeReader is the prompt assembler's read-only view of human-authored
// LYRA.md. Writing and listing knowledge belong to the workspace use case.
type KnowledgeReader interface {
	Get(ctx context.Context, scope knowledge.Scope, dir string) (string, error)
}

// PlanReader is the execution adapter's read-only view of session plan: the
// items for prompt assembly, and the whole projection — revision included — for
// the state.snapshot the turn publishes after a replacement.
// Replacing a list and lifecycle cleanup belong to their direct consumers.
type PlanReader interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
	State(ctx context.Context, sessionID string) (plan.State, error)
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

// CheckpointReader is the execution adapter's read-only durable continuation
// boundary. The complete executor tree remains opaque inside one Application-owned
// aggregate; writes belong to the application transaction that consumes a
// captured checkpoint.
type CheckpointReader interface {
	LoadCheckpoint(ctx context.Context, rootProcessID string) (execution.ExecutorCheckpoint, error)
}

// ToolResolver is the execution adapter's view of model-facing tool groups.
// The interface belongs to this consumer; toolset supplies the implementation
// without importing agentexec.
type ToolResolver interface {
	core.ToolGroupResolver
	UseDelegationTool(toolcontract.Tool)
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

	// UserHome anchors the home-scoped AGENTS.md instruction cascade. The caller
	// resolves it once; the execution adapter never
	// consults ambient user-home state on its own. Empty omits the home layer.
	UserHome string

	// HistoryStore optionally supplies a persistent chat-history backend. When nil the
	// engine falls back to lynx's in-process [in-memory history store]
	// — fine for tests but loses history on restart.
	HistoryStore history.Store

	// Knowledge optionally supplies the human-authored LYRA.md cascade reader.
	// nil disables that prompt layer; curated memory and discovered AGENTS.md
	// remain independent.
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

	// Plan optionally supplies the root Agent's per-session execution plan.
	// The tool resolver owns set_plan visibility; this read-only port only injects
	// the current plan into system prompts. nil disables plan injection.
	Plan PlanReader

	// ToolResolver supplies the execution-time role groups and accepts the task
	// delegation tool that can only be built after the subtask Agent deploys.
	// Catalogs, MCP controls, and shutdown hooks remain outside execution.
	ToolResolver ToolResolver

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

	// Checkpoints restores complete executor checkpoints. nil keeps execution in
	// memory and makes surfaced waiting boundaries unavailable. Captures remain
	// data-only; the Application persists them inside its own atomic write-set.
	Checkpoints CheckpointReader

	// ToolResultStore backs tool-result eviction: a single tool output larger
	// than ToolResultThreshold is offloaded here and replaced in history by a
	// head+tail preview the model can read back via read_tool_result. nil
	// disables eviction (results always flow to history in full).
	ToolResultStore toolResultOffloader

	// ToolResultThreshold is the byte size above which a single tool result is
	// offloaded (see ToolResultStore). Zero or negative disables eviction
	// regardless of ToolResultStore.
	ToolResultThreshold int

	// ToolResultReaderName is the model-facing capability that retrieves an
	// offloaded body. It is required when result eviction is enabled, used both
	// in the preview instruction and to prevent recursively offloading its output.
	ToolResultReaderName string
}
