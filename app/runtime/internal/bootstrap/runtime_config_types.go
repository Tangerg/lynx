package bootstrap

import (
	"context"
	"io"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// Config is the construction-time bundle for [Assemble]. Engine carries the
// engine's own construction config verbatim; the remaining fields are
// the runtime-layer services. Several are required and injected by the
// composition root (the sqlite-backed stores marked "Required" below).
type Config struct {
	// Engine is the Agent execution adapter's construction config. The runtime
	// fills its SessionStore, ProcessStore, Provider, Todos, and ToolResolver.
	Engine agentexec.Config

	// SkillsGlobalDir is the user-scope Agent Skills directory. Tool resolution
	// and workspace discovery consume it directly; it is not Agent execution
	// state.
	SkillsGlobalDir string

	// Turn-boundary collaborators. nil selects the in-house/default binding:
	// conversation steering, LLM compaction, knowledge extraction, and skill
	// mining.
	Steering     turn.SteeringSink
	Compactor    turn.Compactor
	Extractor    turn.Extractor
	Miner        turn.SkillMiner
	SkillCurator turn.SkillCurator

	// AgentMemoryStore is the SQLite fact ledger and its curated memory items,
	// used by the default Extractor and injected into the system prompt. nil
	// disables agent-maintained memory without affecting user-editable LYRA.md.
	AgentMemoryStore *sqlitestore.AgentMemoryStore

	IdempotencyStore *sqlitestore.IdempotencyStore

	// Resources are process adapters whose ownership transfers to Runtime only
	// when [Assemble] succeeds. Close releases them after background tasks and the
	// execution/tool capabilities have stopped; callers retain ownership when
	// construction fails.
	Resources []io.Closer

	// UtilityRoleStore persists the global utility-model role; the (provider,
	// model) the in-house maintenance services (compaction / extraction /
	// titling) run on. nil disables persistence: the role stays unset and those
	// services run on the main turn model. The composition root injects the
	// sqlite-backed store and seeds it from config on first run.
	UtilityRoleStore UtilityRoleStore

	// Tool-environment inputs; the runtime reads these to assemble the tool
	// environment via toolset.Build and inject only its role resolver into the
	// Agent execution adapter.
	Online     OnlineConfig
	A2AAgents  []A2AAgentConfig
	LSPServers []LSPServerConfig

	// SandboxShell opts the shell tools into per-command OS isolation (an
	// in-place jail rooted at the command's cwd: workspace-write only, network
	// denied, $HOME hidden, env scrubbed). Off by default; on a host with no
	// isolation backend it refuses assembly (fail-closed). SandboxReadOnlyPaths
	// re-opens declared toolchain roots below the hidden home for reads.
	SandboxShell         bool
	SandboxReadOnlyPaths []string

	// SandboxDir roots the ephemeral working copies for isolated runs — a session
	// marked Isolated runs its tools in a throwaway copy of its project under
	// this dir. Empty disables isolation (an isolated session's run is then
	// refused, fail-closed). The copies are scratch: never snapshotted.
	SandboxDir string

	// MCPRegistry is the runtime-mutable MCP-server registry. The enabled
	// entries are dialed at boot (the env seed lands here first, in the
	// composition root) and the registry is the source for runtime
	// mcp.configs.configure / remove / setEnabled. Required.
	MCPRegistry mcpserver.Registry

	// SessionStore persists Lyra sessions. Required; the composition root injects
	// the sqlite-backed store (tests use a sqlite :memory: DB) and threads it to
	// the consumers that each hold their own narrow session port — the sessions
	// coordinator, the run-segment titler, and the sub-agent spawn adapter. The
	// concrete type is named here because persistence is single-backend and this
	// is the composition ring (see doc/EXECUTION_CENTERED_ARCHITECTURE.md §8.1).
	SessionStore *sqlitestore.SessionStore

	// RunStore is the durable Run-admission backstop (§8.2): the authoritative
	// "one non-terminal Run per Session" table the run coordinator records
	// admissions/terminals through, and the boot reconcile sweeps. Required: an
	// in-memory-only fallback would violate the restart-safe admission invariant.
	RunStore *sqlitestore.RunStateStore

	// ProcessStore holds the recoverable agent-process snapshot referenced by a
	// parked interrupt. Required so session cancel/delete/rollback can remove the
	// snapshot in the same SQLite write-set as the interrupt and admission row.
	ProcessStore *sqlitestore.ProcessStore

	// WorkspaceMutationStore is the §8.5 recoverable operation log for file
	// rollbacks: the intent recorded before a working-tree + history rollback and
	// cleared once both commit, so a crash is re-driven at boot. nil disables the
	// log (rollback runs best-effort). The composition root injects the
	// sqlite-backed store.
	WorkspaceMutationStore *sqlitestore.WorkspaceMutationStore

	// InterruptStore records open HITL interrupts (R-model resume discovery).
	// Required; injected sqlite-backed, same as SessionStore (concrete for the
	// same single-backend / composition-ring reason).
	InterruptStore *sqlitestore.InterruptStore

	// TranscriptStore persists the durable Item history that items.list is
	// served from (authoritative completed Items + their RunRefs). Required;
	// injected sqlite-backed, same as SessionStore.
	TranscriptStore *sqlitestore.TranscriptStore

	// ProviderRegistry is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required; the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	ProviderRegistry provider.Registry

	// TodoStore persists per-session todo lists for the todo_write tool.
	// Optional; nil disables the feature (no tool, no prompt injection). The
	// composition root injects the sqlite-backed store.
	TodoStore todo.Store

	// GoalStore persists per-session autonomous goals (Goal mode). Optional; nil
	// disables the feature (no update_goal tool, goals.* report
	// capability_not_negotiated). The composition root injects the sqlite store.
	GoalStore goal.Store

	// ApprovalMode sets the initial runtime approval stance. The zero value is
	// [approval.ModeSafe]; [RuntimeConfig] explicitly selects the product default
	// [approval.ModeBalanced]. Unknown modes fail assembly.
	ApprovalMode approval.Mode

	// ApprovalRuleStore persists fine-grained "remember this decision" rules
	// (AUX_API §6). nil is supported for mode-only test environments: Decide
	// never matches and remember/forget return an unavailable error. The product
	// composition root injects the sqlite-backed store.
	ApprovalRuleStore approval.RuleStore

	// Provider / Model name the runtime's DEFAULT provider+model; the one a turn
	// runs against when it doesn't pick a model. providers.list / models.list
	// are served from the registry + catalog, not these.
	Provider string
	Model    string

	// HooksResolver resolves user-configured lifecycle hooks for a turn's cwd.
	// nil disables hooks; the turn no-ops every hook seam. The composition root
	// builds the adapter-backed resolver from the storage home + trust store.
	HooksResolver HookResolver

	// HookTrustStore backs the hooks.* trust toggle (a GUI granting a
	// project's hooks). nil means trust is read-only (CLI / file only); the
	// resolver still reads trust through its own checker.
	HookTrustStore HookTrustStore

	// RecipesGlobalDir is the global recipes directory (<LYRA_HOME>/recipes) the
	// recipes.list discovery layers under a project's .lyra/recipes.
	// Empty means only project recipes are listed. The composition root sets it.
	RecipesGlobalDir string

	// CheckpointDir roots the per-session shadow-git repos backing run-boundary
	// file snapshots (<LYRA_HOME>/checkpoints); the checkpoint adapter enables
	// snapshots + file rollback only when git is present. Empty disables file
	// checkpoints. The composition root sets it.
	CheckpointDir string

	// ScheduleRegistry persists scheduled runs (schedules.*) and is the registry
	// the scheduler worker fires from. nil disables scheduling; schedules.*
	// fails and the worker no-ops. The composition root injects the sqlite-backed
	// store.
	ScheduleRegistry schedule.Registry

	// EmbeddingRoleStore persists the embedding-model role the @codebase index
	// uses (models.setEmbeddingRole). nil disables persistence. CodebaseStore
	// persists the index itself; nil disables the @codebase feature entirely
	// (no tool, no RPC). The composition root injects the sqlite-backed stores.
	EmbeddingRoleStore EmbeddingRoleStore
	CodebaseStore      codebaseindex.Store

	// ToolResultStore persists tool-result bodies offloaded on context eviction
	// (read back by read_tool_result). Injected sqlite-backed for the same
	// single-backend / composition-ring reason as the other concrete stores; the
	// runtime threads its stage/bind view onto execution, its read view onto the
	// tool environment, its portable-blob view onto session I/O, and its cleanup
	// view onto startup reconciliation and the session-delete cascade. nil disables
	// eviction (results always flow to history in full).
	ToolResultStore *sqlitestore.ToolResultStore

	// ToolResultThreshold is the byte size above which a single tool result is
	// offloaded (see ToolResultStore). Zero or negative disables eviction.
	ToolResultThreshold int

	// Transactor runs a write-set inside one storage transaction, so the sessions
	// coordinator's cross-store operations (sessions.import / rollback / delete
	// cascade) commit atomically. Required; the composition root wires the single
	// SQLite backend's transactor into the sessions coordinator.
	Transactor Transactor
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

// A2AAgentConfig identifies one remote Agent-to-Agent endpoint the runtime
// should expose as a delegation tool.
type A2AAgentConfig struct {
	Name              string
	CardURL           string
	AllowedRPCOrigins []string
}

// LSPServerConfig is one optional language-server table entry. Empty
// LSPServers means the runtime falls back to its built-in table.
type LSPServerConfig struct {
	Name        string
	Command     string
	Args        []string
	LanguageID  string
	Extensions  []string
	RootMarkers []string
}

// HookTrustStore mutates project hook trust for the hooks.setTrust
// surface. The sqlite TrustStore implements it.
type HookTrustStore interface {
	Trust(ctx context.Context, projectRoot string) error
	Untrust(ctx context.Context, projectRoot string) error
}

// HookResolver is the runtime's consumer view of lifecycle-hook resolution.
type HookResolver interface {
	For(ctx context.Context, cwd string) (*hooks.Bound, error)
	Inspect(ctx context.Context, cwd string) (hooks.Inspection, error)
}

// Transactor runs fn inside a single storage transaction; the seam the
// composition root uses to give the Runtime cross-store atomicity without
// coupling it to the sqlite backend.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// UtilityRoleStore persists the global utility-model role across restarts. The
// composition root loads it at boot to seed the live cell and injects the
// sqlite-backed implementation. A nil store disables persistence — the role
// stays in-process only. Consumed by bootstrap + the capabilities coordinator.
type UtilityRoleStore interface {
	LoadUtilityRole(ctx context.Context) (provider, model string, err error)
	SaveUtilityRole(ctx context.Context, provider, model string) error
}

// EmbeddingRoleStore persists the embedding-model role across restarts. nil
// disables persistence — the role stays whatever was last set in-process.
type EmbeddingRoleStore interface {
	LoadEmbeddingRole(ctx context.Context) (provider, model string, err error)
	SaveEmbeddingRole(ctx context.Context, provider, model string) error
}
