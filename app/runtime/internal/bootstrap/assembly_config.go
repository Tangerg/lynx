package bootstrap

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/scope/app/runtime/internal/application/approvals"
	"github.com/Tangerg/scope/app/runtime/internal/application/conversations"
	"github.com/Tangerg/scope/app/runtime/internal/application/goals"
	mcpapp "github.com/Tangerg/scope/app/runtime/internal/application/mcp"
	"github.com/Tangerg/scope/app/runtime/internal/application/models"
	"github.com/Tangerg/scope/app/runtime/internal/application/ownershiprecovery"
	"github.com/Tangerg/scope/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/scope/app/runtime/internal/domain/approval"
	"github.com/Tangerg/scope/app/runtime/internal/infra/mcp"
	sqlitestore "github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chatclient"
)

// Config is the construction-time bundle for [NewAssembly]. It contains Host
// capabilities and application adapters only; Bootstrap derives the
// Agent Framework executor configuration so no second source of Runtime facts exists.
type Config struct {
	// BuildID identifies the running executable at durable executor boundaries.
	BuildID string

	// SessionOwnership extends Run/session lifecycle and destructive working-tree
	// admission across Runtime processes sharing one data directory.
	SessionOwnership sessionadmission.Ownership
	// GoalDriveOwnership elects one autonomous Goal driver per Session across
	// those Runtime processes.
	GoalDriveOwnership goals.DriveOwnership
	// RecoveryOwnership elects one process to reconcile abandoned Runs before
	// Goals, preserving their accounting order across shared Runtime instances.
	RecoveryOwnership ownershiprecovery.Ownership

	// ChatClient is the default model client. Explicit per-Run selections resolve
	// through ProviderRegistry; utility roles fall back to this client.
	ChatClient *chatclient.Client

	// ConversationStore is the authoritative model-context store.
	ConversationStore conversations.Store

	// Pricing computes model usage cost for Runtime projections.
	Pricing accounting.Pricing

	// SkillsUserDir is the user-scope Agent Skills directory. Tool resolution
	// and workspace discovery consume it directly; it is not Agent execution
	// state.
	SkillsUserDir string

	// Maintenance overrides the default post-Run maintenance pipeline.
	Maintenance agentexec.RunMaintenance

	// AgentMemoryStore is the SQLite fact ledger and its curated memory items,
	// used by the default MemoryConsolidator and injected into the system prompt. nil
	// disables agent-maintained memory without affecting user-editable LYRA.md.
	AgentMemoryStore *sqlitestore.AgentMemoryStore

	IdempotencyStore *sqlitestore.IdempotencyStore

	// Resources are one-shot process adapters whose ownership transfers to
	// Assembly when [NewAssembly] is called. A successful [BuildAssembly]
	// transfers them to Host. Host bounds each Close call and releases resources
	// only after background tasks and execution/tool capabilities have stopped;
	// a returned error is retained as a diagnostic but cannot make a terminal
	// Close replayable.
	Resources []TerminalResource

	// UtilityRoleStore persists the global utility-model role used by history
	// compaction, memory consolidation, Skill proposal mining, and Session title
	// generation. nil disables persistence: the role stays unset and those calls
	// use the main Run model. The composition root injects the
	// sqlite-backed store and seeds it from config on first run.
	UtilityRoleStore UtilityRoleStore

	// Tool-environment inputs; the runtime reads these to assemble the tool
	// environment via toolset.Build and inject only its role resolver into the
	// Agent execution adapter.
	Online     toolset.OnlineConfig
	A2AAgents  []toolset.A2AAgentConfig
	LSPServers []codeintel.ServerSpec

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
	// mcp.servers.create / update / delete. Required.
	MCPRegistry mcpapp.Registry

	// MCPOAuthSessions persists refreshing OAuth credentials independently of
	// the domain registry shape. Required: without it desktop sign-in would be
	// process-local and every restart would unnecessarily re-authorize.
	MCPOAuthSessions mcp.OAuthSessionStore

	// SessionStore persists Lyra sessions. Required; the composition root injects
	// the sqlite-backed store (tests use a sqlite :memory: DB) and threads it to
	// the consumers that each hold their own narrow session port — the sessions
	// coordinator, the Session title generator, and the child-Run admission adapter. The
	// concrete type is named here because persistence is single-backend and this
	// is the composition ring (see doc/ARCHITECTURE.md).
	SessionStore *sqlitestore.SessionStore

	// RunStore is the durable Run table (§8.2): the authoritative "one
	// non-terminal Run per Session" admission backstop through which the Run
	// coordinator records admissions and terminal states, the record that answers
	// every Run read,
	// and what the boot reconcile sweeps. Required: an in-memory-only fallback
	// would violate the restart-safe admission invariant.
	RunStore *sqlitestore.RunStore

	// Invocation journals and child-start reservations close the executor's
	// durable side-effect and admission boundaries. Child-start callback receipts
	// live only for their active root tree/process and are reclaimed by terminal,
	// Session lifecycle, and boot-recovery write-sets. Required.
	ModelInvocationStore *sqlitestore.ModelInvocationStore
	ToolInvocationStore  *sqlitestore.ToolInvocationStore
	ChildRunStartStore   *sqlitestore.ChildRunStartReservationStore

	// ExecutorCheckpoints stores the opaque, root-owned executor continuation
	// referenced by a parked interrupt. Required so lifecycle write-sets can save
	// or remove that continuation atomically without interpreting its payload.
	ExecutorCheckpoints *persistence.ExecutorCheckpointStore

	// WorkspaceMutationStore is the §8.5 recoverable operation log for file
	// rollbacks: the intent recorded before a working-tree + history rollback and
	// cleared once both commit, so a crash is re-driven at boot. nil disables the
	// log (rollback runs best-effort). The composition root injects the
	// sqlite-backed store.
	WorkspaceMutationStore *persistence.WorkspaceMutationStore

	// InterruptStore records open HITL interrupts (R-model resume discovery).
	// Required; injected sqlite-backed, same as SessionStore (concrete for the
	// same single-backend / composition-ring reason).
	InterruptStore *persistence.InterruptStore

	// TranscriptStore persists the durable Item history that items.list is
	// served from (authoritative completed Items + their RunRefs). Required;
	// injected sqlite-backed, same as SessionStore.
	TranscriptStore *sqlitestore.TranscriptStore

	// FeedbackStore retains feedback.create quality signals. Required: the
	// protocol acknowledges feedback only after this durable receiver accepts it.
	FeedbackStore *sqlitestore.FeedbackStore

	// ProviderRegistry is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required; the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	ProviderRegistry models.ProviderRegistry

	// PlanStore persists per-session Plan aggregates through application-owned
	// optimistic replacements. Optional; nil disables the feature (no tool, no
	// prompt injection). The composition root injects the sqlite-backed store.
	PlanStore PlanStore

	// PermissionModeStore persists session-scoped Plan-mode entry and the exact
	// permission mode restored on exit. Optional only for tests that do not expose
	// Plan-mode tools; product composition always supplies SQLite.
	PermissionModeStore approvals.ModeStore

	// GoalStore persists per-session autonomous goals (Goal mode). Optional; nil
	// disables the feature (no Goal tools; goals.* report
	// capability_not_negotiated). The composition root injects the sqlite store.
	GoalStore goals.DurableStore

	// KnowledgeStore persists the human-authored LYRA.md cascade for both the
	// workspace use case and the execution adapter's read-only prompt view.
	KnowledgeStore workspace.KnowledgeStore
	// KnowledgeDirectory is the explicit global filesystem scope observed for
	// externally-authored knowledge changes. It is the Runtime data directory,
	// not the OS user home used by global hook configuration.
	KnowledgeDirectory string

	// ApprovalMode sets the initial runtime approval stance. It must be explicit;
	// [ComposeConfig] selects the product default [approval.ModeBalanced]. The
	// empty or an unknown mode fails assembly.
	ApprovalMode approval.Mode

	// ApprovalRuleStore persists fine-grained "remember this decision" rules
	// (AUX_API §6). nil is supported for mode-only test environments: Decide
	// never matches and remember/forget return an unavailable error. The product
	// composition root injects the sqlite-backed store.
	ApprovalRuleStore ApprovalRuleStore

	// Provider / Model name the runtime's default provider+model; the one a Run
	// runs against when it doesn't pick a model. providers.list / models.list
	// are served from the registry + catalog, not these.
	Provider string
	Model    string

	// HooksResolver resolves user-configured lifecycle hooks for a Run's cwd.
	// nil disables hooks; execution no-ops every hook seam. The composition root
	// builds the adapter-backed resolver from the storage home + trust store.
	HooksResolver HookResolver

	// HookTrustStore backs the hooks.* trust toggle (a GUI granting a
	// project's hooks). nil means trust is read-only (CLI / file only); the
	// resolver still reads trust through its own checker.
	HookTrustStore workspace.HookTrustStore

	// RecipesGlobalDir is the global recipes directory (<LYRA_HOME>/recipes) the
	// recipes.list discovery layers under a project's .lyra/recipes.
	// Empty means only project recipes are listed. The composition root sets it.
	RecipesGlobalDir string

	// CheckpointDir roots the per-session shadow-git repos backing run-boundary
	// file snapshots (<LYRA_HOME>/checkpoints); the checkpoint adapter enables
	// snapshots + file rollback only when git is present. Empty disables file
	// checkpoints. The composition root sets it.
	CheckpointDir string

	// ScheduleStore persists scheduled runs (schedules.*), serving management,
	// run-now, and the scheduler worker. nil disables scheduling; schedules.*
	// fails and the worker no-ops. The composition root injects the sqlite-backed
	// store.
	ScheduleStore ScheduleStore

	// UserHome is the process user's home directory. It anchors home-scoped
	// instruction discovery and is resolved once by the outer process root.
	UserHome string

	// DefaultWorkspacePath is the workspace selected when a request or saved
	// schedule does not name one. It is a product default supplied by the outer
	// process root, not the server process's current working directory.
	DefaultWorkspacePath string

	// EmbeddingRoleStore persists the optional embedding model used to enrich
	// agent-memory ranking. nil disables role persistence; memory remains
	// keyword-searchable. The composition root injects the SQLite-backed store.
	EmbeddingRoleStore EmbeddingRoleStore

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

// validateAssemblyConfig reports whether the construction-time bundle has
// every capability required to assemble a Runtime. Bootstrap keeps this as a
// construction function: Config is data at the composition boundary, not a
// domain object with business behavior.
func validateAssemblyConfig(c Config) error {
	if c.UserHome == "" {
		return errors.New("runtime: UserHome is required")
	}
	if !filepath.IsAbs(c.UserHome) {
		return errors.New("runtime: UserHome must be absolute")
	}
	if c.DefaultWorkspacePath == "" {
		return errors.New("runtime: DefaultWorkspacePath is required")
	}
	if !filepath.IsAbs(c.DefaultWorkspacePath) {
		return errors.New("runtime: DefaultWorkspacePath must be absolute")
	}
	if c.KnowledgeDirectory == "" {
		return errors.New("runtime: KnowledgeDirectory is required")
	}
	for _, configuredPath := range []struct {
		name  string
		value string
	}{
		{name: "SkillsUserDir", value: c.SkillsUserDir},
		{name: "SandboxDir", value: c.SandboxDir},
		{name: "RecipesGlobalDir", value: c.RecipesGlobalDir},
		{name: "CheckpointDir", value: c.CheckpointDir},
		{name: "KnowledgeDirectory", value: c.KnowledgeDirectory},
	} {
		if configuredPath.value != "" && !filepath.IsAbs(configuredPath.value) {
			return fmt.Errorf("runtime: %s must be absolute when set", configuredPath.name)
		}
	}
	for index, configuredPath := range c.SandboxReadOnlyPaths {
		if configuredPath != "" && !filepath.IsAbs(configuredPath) {
			return fmt.Errorf("runtime: SandboxReadOnlyPaths[%d] must be absolute when set", index)
		}
	}
	if c.ChatClient == nil {
		return errors.New("runtime: ChatClient is required")
	}
	if c.BuildID == "" {
		return errors.New("runtime: BuildID is required")
	}
	if c.ConversationStore == nil {
		return errors.New("runtime: ConversationStore is required")
	}
	if c.ProviderRegistry == nil {
		return errors.New("runtime: ProviderRegistry is required")
	}
	if c.MCPRegistry == nil {
		return errors.New("runtime: MCPRegistry is required")
	}
	if c.MCPOAuthSessions == nil {
		return errors.New("runtime: MCPOAuthSessions is required")
	}
	if c.SessionStore == nil {
		return errors.New("runtime: SessionStore is required")
	}
	if c.InterruptStore == nil {
		return errors.New("runtime: InterruptStore is required")
	}
	if c.TranscriptStore == nil {
		return errors.New("runtime: TranscriptStore is required")
	}
	if c.FeedbackStore == nil {
		return errors.New("runtime: FeedbackStore is required")
	}
	if c.RunStore == nil {
		return errors.New("runtime: RunStore is required")
	}
	if c.ExecutorCheckpoints == nil {
		return errors.New("runtime: ExecutorCheckpoints is required")
	}
	if c.ModelInvocationStore == nil {
		return errors.New("runtime: ModelInvocationStore is required")
	}
	if c.ToolInvocationStore == nil {
		return errors.New("runtime: ToolInvocationStore is required")
	}
	if c.ChildRunStartStore == nil {
		return errors.New("runtime: ChildRunStartStore is required")
	}
	if c.Transactor == nil {
		return errors.New("runtime: Transactor is required")
	}
	return nil
}
