package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// Config is the construction-time bundle for [New]. Engine carries the
// engine's own construction config verbatim; the remaining fields are
// the runtime-layer services. Several are required and injected by the
// composition root (the sqlite-backed stores marked "Required" below).
type Config struct {
	// Engine is the engine's construction config. The runtime fills its
	// SessionStore (adapted from the Lyra session store) and the
	// tool-environment fields (ToolResolver/Tools/MCP/Closers) from
	// [toolset.Build] below; Engine.ChatClient is required.
	Engine kernel.Config

	// UtilityRoleStore persists the global utility-model role; the (provider,
	// model) the in-house maintenance services (compaction / extraction /
	// titling) run on. nil disables persistence: the role stays unset and those
	// services run on the main turn model. The composition root injects the
	// sqlite-backed store and seeds it from config on first run.
	UtilityRoleStore UtilityRoleStore

	// Tool-environment inputs; the runtime reads these to assemble the tool
	// environment via toolset.Build and inject it into the engine core (which
	// constructs no capability itself). Workdir / SkillsGlobalDir come from
	// Engine (the engine also needs them for the prompt cascade / listSkills).
	Online     OnlineConfig
	A2AAgents  []A2AAgentConfig
	LSPServers []LSPServerConfig

	// MCPRegistry is the runtime-mutable MCP-server registry. The enabled
	// entries are dialed at boot (the env seed lands here first, in the
	// composition root) and the registry is the source for runtime
	// workspace.mcp.configure / remove / setEnabled. Required.
	MCPRegistry mcpserver.Registry

	// SessionStore persists Lyra sessions. Required; the composition root
	// injects the sqlite-backed store (tests use a sqlite :memory: DB).
	SessionStore sessionsvc.Store

	// InterruptStore records open HITL interrupts (R-model resume discovery).
	// Required; injected sqlite-backed, same as SessionStore.
	InterruptStore interrupts.Store

	// TranscriptStore persists the durable Item history that items.list is
	// served from (authoritative completed Items + their RunRefs). Required;
	// injected sqlite-backed, same as SessionStore.
	TranscriptStore transcript.Store

	// ProviderRegistry is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required; the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	ProviderRegistry provider.Registry

	// TodoStore persists per-session todo lists for the todo_write tool.
	// Optional; nil disables the feature (no tool, no prompt injection). The
	// composition root injects the sqlite-backed store.
	TodoStore todo.Store

	// ApprovalMode sets the initial runtime approval stance. The service is
	// always constructed; mode defaults to [approval.ModeYolo] when this field is
	// the zero value.
	ApprovalMode approval.Mode

	// ApprovalRuleStore persists fine-grained "remember this decision" rules
	// (AUX_API §6). nil means no rules are remembered (Decide never matches);
	// the composition root injects the sqlite-backed store.
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

	// HookTrustStore backs the workspace.hooks.* trust toggle (a GUI granting a
	// project's hooks). nil means trust is read-only (CLI / file only); the
	// resolver still reads trust through its own checker.
	HookTrustStore HookTrustStore

	// RecipesGlobalDir is the global recipes directory (<LYRA_HOME>/recipes) the
	// workspace.recipes.list discovery layers under a project's .lyra/recipes.
	// Empty means only project recipes are listed. The composition root sets it.
	RecipesGlobalDir string

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

	// Transactor runs a write-set inside one storage transaction, so the
	// cross-store operations (sessions.import / rollback) commit atomically. nil
	// means [Runtime.runInTx] runs the function directly (no atomicity), keeping
	// non-sqlite / test runtimes working. The composition root wires the
	// sqlite-backed transactor.
	Transactor Transactor
}

// OnlineConfig holds credentials for optional network-reaching tools. Empty
// fields leave the corresponding tool disabled.
type OnlineConfig struct {
	JinaAPIKey       string
	TavilyAPIKey     string
	HTTPAllowedHosts []string
}

// A2AAgentConfig identifies one remote Agent-to-Agent endpoint the runtime
// should expose as a delegation tool.
type A2AAgentConfig struct {
	Name    string
	CardURL string
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

// HookTrustStore mutates project hook trust for the workspace.hooks.setTrust
// surface. The sqlite TrustStore implements it.
type HookTrustStore interface {
	Trust(ctx context.Context, projectRoot string) error
	Untrust(ctx context.Context, projectRoot string) error
}

// HookResolver is the runtime's consumer view of lifecycle-hook resolution.
type HookResolver interface {
	For(ctx context.Context, cwd string) *hooks.Bound
	Inspect(ctx context.Context, cwd string) hooks.Inspection
}

// Transactor runs fn inside a single storage transaction; the seam the
// composition root uses to give the Runtime cross-store atomicity without
// coupling it to the sqlite backend.
type Transactor func(ctx context.Context, fn func(context.Context) error) error
