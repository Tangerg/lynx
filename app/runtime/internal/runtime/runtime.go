// Package runtime is Lyra's core-runtime façade — one struct that
// bundles the kernel + every domain service a transport adapter
// might need. The architecture goal documented in GREENFIELD_ARCHITECTURE.md is
// "transport-agnostic Service interface": Runtime is that interface,
// realized in code.
//
// Decoupling boundary:
//
//	cmd/lyra ──┐
//	           │ build
//	           ▼
//	    runtime.Runtime  ◄──── transport adapters
//	           ▲                 (HTTP, IPC, gRPC, MCP)
//	           │ owns
//	           ▼
//	    kernel + domain/*  (in-process implementations)
//
// Today the runtime + all transports live in the same Go process. The
// boundary still matters: transports depend on runtime, not on the
// concrete service constructors, so a future "remote" runtime impl
// (one process for the kernel, another for the transport) only needs
// to satisfy [Runtime]'s accessor surface.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/core/model/chat/history"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
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

	// UtilityRoleStore persists the global utility-model role — the (provider,
	// model) the in-house maintenance services (compaction / extraction /
	// titling) run on. nil disables persistence: the role stays unset and those
	// services run on the main turn model. The composition root injects the
	// sqlite-backed store and seeds it from config on first run.
	UtilityRoleStore UtilityRoleStore

	// Tool-environment inputs — the runtime reads these to assemble the tool
	// environment via toolset.Build and inject it into the engine core (which
	// constructs no capability itself). Workdir / SkillsGlobalDir come from
	// Engine (the engine also needs them for the prompt cascade / listSkills).
	Online     toolset.OnlineConfig   // network-tool credentials
	A2AAgents  []a2a.ClientConfig     // remote A2A agents to dial
	LSPServers []codeintel.ServerSpec // language-server table (nil → defaults)

	// MCPRegistry is the runtime-mutable MCP-server registry. The enabled
	// entries are dialed at boot (the env seed lands here first, in the
	// composition root) and the registry is the source for runtime
	// workspace.mcp.configure / remove / setEnabled. Required.
	MCPRegistry mcpserver.Registry

	// SessionStore persists Lyra sessions. Required — the composition root
	// injects the sqlite-backed store (tests use a sqlite :memory: DB).
	SessionStore sessionsvc.Store

	// InterruptStore records open HITL interrupts (R-model resume
	// discovery). Required — injected sqlite-backed, same as SessionStore.
	InterruptStore interrupts.Store

	// TranscriptStore persists the durable Item history that items.list is
	// served from (authoritative completed Items + their RunRefs).
	// Required — injected sqlite-backed, same as SessionStore.
	TranscriptStore transcript.Store

	// ProviderRegistry is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required — the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	ProviderRegistry provider.Registry

	// TodoStore persists per-session todo lists for the todo_write tool.
	// Optional — nil disables the feature (no tool, no prompt injection). The
	// composition root injects the sqlite-backed store.
	TodoStore todo.Store

	// ApprovalMode sets the initial runtime approval stance. The
	// service is always constructed; mode defaults to [approval.ModeYolo]
	// when this field is the zero value.
	ApprovalMode approval.Mode

	// ApprovalRuleStore persists fine-grained "remember this decision" rules
	// (AUX_API §6). nil → no rules are remembered (Decide never matches); the
	// composition root injects the sqlite-backed store.
	ApprovalRuleStore approval.RuleStore

	// Provider / Model name the runtime's DEFAULT provider+model — the one a
	// turn runs against when it doesn't pick a model. providers.list /
	// models.list are served from the registry + catalog, not these.
	Provider string
	Model    string

	// HooksResolver resolves user-configured lifecycle hooks for a turn's cwd.
	// nil disables hooks — the turn no-ops every hook seam. The composition root
	// builds the adapter-backed resolver from the storage home + trust store.
	HooksResolver HookResolver

	// HookTrustStore backs the workspace.hooks.* trust toggle (a GUI granting a
	// project's hooks). nil → trust is read-only (CLI / file only); the resolver
	// still reads trust through its own checker.
	HookTrustStore HookTrustStore

	// RecipesGlobalDir is the global recipes directory (<LYRA_HOME>/recipes) the
	// workspace.recipes.list discovery layers under a project's .lyra/recipes.
	// Empty → only project recipes are listed. The composition root sets it.
	RecipesGlobalDir string

	// ScheduleRegistry persists scheduled runs (schedules.*) and is the registry
	// the scheduler worker fires from. nil disables scheduling — schedules.*
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
	// → [Runtime.RunInTx] runs the function directly (no atomicity), keeping
	// non-sqlite / test runtimes working. The composition root wires the
	// sqlite-backed transactor.
	Transactor Transactor
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

// Runtime is the bundle. Construct once via [New]; share the
// pointer across every transport adapter that needs to dispatch
// turns / sessions / approvals.
//
// Concurrency: every accessor returns a Service whose own methods are safe for
// concurrent use. Runtime owns the process-local coordination state that
// defines application lifecycle invariants across transports.
type Runtime struct {
	engine     *kernel.Engine
	turnSvc    turn.Service
	session    sessionsvc.Store
	tool       toolsvc.Service
	knowledge  knowledge.Store
	approval   approval.Service
	interrupts interrupts.Store
	transcript transcript.Store

	// conversation is the message history the non-turn history ops
	// (ReadHistory/SeedHistory/MessageCount/TruncateMessages) delegate to
	// directly — not via the engine (it owns only the steering touchpoint).
	conversation *conversation.Messages

	providers   provider.Registry
	mcpRegistry mcpserver.Registry

	// mcpGating holds the current per-call MCP tool gating (disabled / auto-
	// approve sets), recomputed on every registry change. The resolver (disabled
	// filter) and the turn gate (auto-approve skip) read it via closures that
	// close over this same cell, captured at construction before the Runtime
	// exists — hence a pointer. See [mcpGating] and [Runtime.refreshMCPGating].
	mcpGating *atomic.Pointer[mcpGating]

	defaultProvider string
	defaultModel    string

	// titler auto-names an untitled session from its first user message — a
	// turn-boundary maintenance op (like the Compactor) on the utility model,
	// triggered by the delivery layer off a finished root run.
	titler *maintenance.Titler

	// utility holds the live utility-model role (provider, model) the
	// maintenance services resolve against; SetUtilityRole repoints it. resolver
	// builds + caches the client for a (provider, model); utilStore persists the
	// role across restarts. See utility.go.
	utility   *atomic.Pointer[utilityRole]
	resolver  *clientResolver
	utilStore UtilityRoleStore

	// hookResolver inspects lifecycle hooks for a cwd (workspace.hooks.list);
	// hookTrust mutates project trust (workspace.hooks.setTrust). Both nil when
	// hooks are unconfigured.
	hookResolver HookResolver
	hookTrust    HookTrustStore

	// recipesGlobalDir is the global recipes directory the workspace.recipes.list
	// discovery layers under a project's .lyra/recipes. Empty → project-only.
	recipesGlobalDir string

	// schedules is the scheduled-run registry (schedules.* + the scheduler
	// worker). nil when scheduling is unconfigured.
	schedules schedule.Registry

	// @codebase semantic index: embeddingCell holds the live embedding role,
	// embeddings builds+caches embedders from it, embeddingStore persists it, and
	// codebaseIndex is the index (nil when no CodebaseStore). See
	// embedding.go.
	embeddingCell  *atomic.Pointer[embeddingRole]
	embeddings     *embeddingResolver
	embeddingStore EmbeddingRoleStore
	codebaseIndex  codebaseindex.Index

	// transactor runs a write-set inside one storage transaction so the
	// cross-store operations (sessions.import / rollback) are atomic; nil → run
	// directly (RunInTx). See [Transactor].
	transactor Transactor

	// workingTrees coordinates short run admissions with destructive
	// working-tree mutations for every transport using this runtime.
	workingTrees lifecycle.WorkingTreeGate
}

// Transactor runs fn inside a single storage transaction — the seam the
// composition root uses to give the Runtime cross-store atomicity without
// coupling it to the sqlite backend.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// runInTx runs fn inside one storage transaction (commit on success, rollback
// on error), so a multi-step write-set across the domain services commits
// atomically. Falls back to running fn directly when no transactor is wired (a
// non-sqlite / test runtime) — correct but without all-or-nothing.
func (r *Runtime) runInTx(ctx context.Context, fn func(context.Context) error) error {
	if r == nil || r.transactor == nil {
		return fn(ctx)
	}
	return r.transactor(ctx, fn)
}

// New assembles a Runtime from cfg. Returns an error when a required
// dependency is missing or any internal constructor fails — engine
// deployment, MCP dial, etc.
func New(ctx context.Context, cfg Config) (*Runtime, error) {
	if cfg.Engine.ChatClient == nil {
		return nil, errors.New("runtime: Engine.ChatClient is required")
	}
	if cfg.TranscriptStore == nil {
		return nil, errors.New("runtime: TranscriptStore is required")
	}

	// The engine config passes through except SessionStore + the microkernel
	// ports. SessionStore: a spawned sub-agent (the `task` delegation) gets its
	// session recorded so the parent→child lineage is durably queryable.
	ecfg := cfg.Engine
	ecfg.SessionStore = newChildSessionStore(cfg.SessionStore)
	// The default provider id — the engine's pricing fallback for a default /
	// subtask turn that names no provider (so its cost attributes to the right
	// provider rather than an alphabetical catalog guess).
	ecfg.Provider = cfg.Provider

	// Microkernel port wiring: the runtime is the composition root that builds
	// the capability implementations and injects them into the engine core
	// (which depends only on the port interfaces). All share one chat-history
	// store so the engine's chat-history middleware and these ports agree.
	historyStore := cfg.Engine.HistoryStore
	if historyStore == nil {
		historyStore = history.NewInMemoryStore()
		ecfg.HistoryStore = historyStore
	}
	// conv is the message history. The engine gets it ONLY as the
	// turn-end steering sink (engine.InjectUserMessage); the runtime holds it
	// directly for the non-turn history operations (read/seed/count/truncate,
	// for fork / rollback / messages.list) rather than proxying them through the
	// engine. See doc/GREENFIELD_ARCHITECTURE.md.
	conv := conversation.NewMessages(historyStore)

	// Capability ports are SPIs: the engine consumes interfaces (Steering /
	// Compactor / Extractor; Knowledge above). The runtime supplies the
	// in-house implementations ONLY when the composition root didn't inject one,
	// so an external provider (e.g. a mem0 / HTTP-bridged compactor or knowledge
	// store) can be slotted in by setting the corresponding engine.Config field —
	// the runtime then leaves it untouched. nil → in-house default.
	// The clientResolver builds a chat client for an explicit (provider, model)
	// from that provider's registry credentials, caching by the credential
	// tuple. A turn uses it to honor a per-run model; the maintenance services
	// below use it to honor the utility-model role.
	providers := cfg.ProviderRegistry
	resolver := newClientResolver(providers)

	utilityEnv, err := buildUtilityEnvironment(ctx, cfg, resolver)
	if err != nil {
		return nil, err
	}
	embeddingEnv, err := buildEmbeddingEnvironment(ctx, cfg, providers)
	if err != nil {
		return nil, err
	}

	if ecfg.Steering == nil {
		ecfg.Steering = conv
	}
	wireMaintenancePorts(&ecfg, cfg, historyStore, utilityEnv.resolve)
	// Todo list: same nil-default contract — honor a pre-injected engine
	// Todos (an external task store), else use the runtime-supplied one.
	if ecfg.Todos == nil {
		ecfg.Todos = cfg.TodoStore
	}

	// Tool environment: assembled outside the core (constructs the code-intel /
	// exec / MCP / A2A capabilities + the resolver) and injected, so the engine
	// core builds no capability. ctx flows so a slow MCP/A2A dial can be
	// canceled during startup.
	// Approval stance is built early: the toolset's exit_plan_mode tool needs it
	// (it flips the stance to execute when a plan is approved), and the turn gate
	// reads it per tool call.
	approvalSvc := approval.New(cfg.ApprovalMode, cfg.ApprovalRuleStore)

	mcpEnv, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return nil, err
	}

	built, err := buildToolEnvironment(ctx, cfg, ecfg, approvalSvc, mcpEnv, embeddingEnv.index)
	if err != nil {
		return nil, err
	}
	ecfg.ToolResolver = built.Resolver
	ecfg.Tools = built.Tools
	ecfg.MCP = built.MCP
	ecfg.Closers = built.Closers

	eng, err := kernel.New(ctx, ecfg)
	if err != nil {
		// toolset.Build already dialed MCP/A2A + launched LSP/exec backends into
		// built.Closers; kernel.New didn't take ownership (no engine to Close), so
		// release them here rather than leaking the sessions/processes.
		runClosers(built.Closers)
		return nil, fmt.Errorf("runtime: engine: %w", err)
	}
	// From here the engine owns built.Closers (eng.Close runs them), so a later
	// construction failure tears down via eng.Close.

	// session / interrupt / provider are required and injected by the
	// composition root (cmd/lyra wires sqlite-backed stores; tests wire a
	// sqlite :memory: DB). The runtime keeps no in-memory fallback — there's
	// a single storage backend now.
	sessionSvc := cfg.SessionStore
	interruptStore := cfg.InterruptStore

	chatSvc, err := turn.New(turn.Dependencies{
		Engine:         eng,
		Approval:       approvalSvc,
		ClientResolver: resolver,
		Todos:          ecfg.Todos,
		MCPAutoApprove: mcpEnv.autoApprove,
		Hooks:          cfg.HooksResolver,
	})
	if err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("runtime: chat service: %w", err)
	}
	toolSvc, err := toolsvc.New(eng)
	if err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("runtime: tool service: %w", err)
	}

	return &Runtime{
		engine:           eng,
		turnSvc:          chatSvc,
		session:          sessionSvc,
		tool:             toolSvc,
		knowledge:        cfg.Engine.Knowledge,
		approval:         approvalSvc,
		interrupts:       interruptStore,
		transcript:       cfg.TranscriptStore,
		conversation:     conv,
		providers:        providers,
		mcpRegistry:      cfg.MCPRegistry,
		mcpGating:        mcpEnv.gate,
		defaultProvider:  cfg.Provider,
		defaultModel:     cfg.Model,
		titler:           maintenance.NewTitler(utilityEnv.resolve),
		utility:          utilityEnv.cell,
		resolver:         resolver,
		utilStore:        cfg.UtilityRoleStore,
		hookResolver:     cfg.HooksResolver,
		hookTrust:        cfg.HookTrustStore,
		recipesGlobalDir: cfg.RecipesGlobalDir,
		schedules:        cfg.ScheduleRegistry,
		embeddingCell:    embeddingEnv.cell,
		embeddings:       embeddingEnv.resolver,
		embeddingStore:   cfg.EmbeddingRoleStore,
		codebaseIndex:    embeddingEnv.index,
		transactor:       cfg.Transactor,
	}, nil
}

// runClosers runs capability shutdown hooks best-effort — used to release a
// half-built tool environment when runtime construction fails before the engine
// (which would otherwise own them) is created.
func runClosers(closers []func() error) {
	for _, closeFn := range closers {
		if closeFn != nil {
			_ = closeFn()
		}
	}
}

func (r *Runtime) forgetSession(sessionID string) { r.turnSvc.ForgetSession(sessionID) }

// DefaultModel is the model a turn runs against when it doesn't pick one
// (the configured Config.Model seed). The session layer uses it to fill
// Session.model for sessions that never explicitly selected a model, so the
// wire always carries a real model name. May be empty if unconfigured.
func (r *Runtime) DefaultModel() string { return r.defaultModel }

// DefaultProvider is the provider a turn runs against when a run names none
// (paired with DefaultModel). usage.summary uses it to attribute default-model
// runs (whose RunRef carries no provider) to the real provider. May be empty.
func (r *Runtime) DefaultProvider() string { return r.defaultProvider }

// ListSkills enumerates the skills visible from cwd (project over global) for
// workspace.listSkills. Delegates to the engine, which owns skill sourcing.
func (r *Runtime) ListSkills(ctx context.Context, cwd string) ([]kernel.SkillInfo, error) {
	return r.engine.ListSkills(ctx, cwd)
}
