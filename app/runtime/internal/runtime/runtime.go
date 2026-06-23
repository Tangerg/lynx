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

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Config is the construction-time bundle for [New]. Engine carries the
// engine's own construction config verbatim; the remaining fields are
// the runtime-layer services. Several are required and injected by the
// composition root (the sqlite-backed stores marked "Required" below).
type Config struct {
	// Engine is the engine's construction config. The runtime fills its
	// SessionStore (derived from SessionService) and the tool-environment
	// fields (ToolResolver/Tools/MCP/Closers) from [toolset.Build] below;
	// Engine.ChatClient is required.
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
	Online     kernel.OnlineConfig    // network-tool credentials
	A2AAgents  []a2a.ClientConfig     // remote A2A agents to dial
	LSPServers []codeintel.ServerSpec // language-server table (nil → defaults)

	// MCPRegistry is the runtime-mutable MCP-server registry. The enabled
	// entries are dialed at boot (the env seed lands here first, in the
	// composition root) and the registry is the source for runtime
	// workspace.mcp.configure / remove / setEnabled. Required.
	MCPRegistry mcpserver.Service

	// SessionService persists Lyra sessions. Required — the composition
	// root injects the sqlite-backed service (tests use a sqlite :memory: DB).
	SessionService sessionsvc.Service

	// InterruptStore records open HITL interrupts (R-model resume
	// discovery). Required — injected sqlite-backed, same as SessionService.
	InterruptStore interrupts.Store

	// TranscriptStore persists the durable Item history that items.list is
	// served from (authoritative completed Items + their RunRefs).
	// Required — injected sqlite-backed, same as SessionService.
	TranscriptStore transcript.Store

	// ProviderService is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required — the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	ProviderService provider.Service

	// TodoService persists per-session todo lists for the todo_write tool.
	// Optional — nil disables the feature (no tool, no prompt injection). The
	// composition root injects the sqlite-backed service.
	TodoService todo.Service

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
	// nil (or a nil *hooks.Resolver) disables hooks — the turn no-ops every
	// hook seam. The composition root builds it from the storage home + the
	// project-trust store.
	HooksResolver *hooks.Resolver

	// HookTrustStore backs the workspace.hooks.* trust toggle (a GUI granting a
	// project's hooks). nil → trust is read-only (CLI / file only); the resolver
	// still reads trust through its own checker.
	HookTrustStore HookTrustStore

	// RecipesGlobalDir is the global recipes directory (<LYRA_HOME>/recipes) the
	// workspace.recipes.list discovery layers under a project's .lyra/recipes.
	// Empty → only project recipes are listed. The composition root sets it.
	RecipesGlobalDir string

	// ScheduleStore persists scheduled runs (schedules.*) and is the registry the
	// scheduler worker fires from. nil disables scheduling — schedules.* fails and
	// the worker no-ops. The composition root injects the sqlite-backed store.
	ScheduleStore schedule.Service

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

// Runtime is the bundle. Construct once via [New]; share the
// pointer across every transport adapter that needs to dispatch
// turns / sessions / approvals.
//
// Concurrency: every accessor returns a Service whose own methods
// are safe for concurrent use. Runtime itself holds no mutable
// state after construction.
type Runtime struct {
	engine     *kernel.Engine
	chat       turn.Service
	session    sessionsvc.Service
	tool       toolsvc.Service
	knowledge  knowledge.Service
	approval   approval.Service
	interrupts interrupts.Store
	transcript transcript.Store

	// conversation is the message-history service the non-turn history ops
	// (ReadHistory/SeedHistory/MessageCount/TruncateMessages) delegate to
	// directly — not via the engine (it owns only the steering touchpoint).
	conversation *conversation.Service

	providers   provider.Service
	mcpRegistry mcpserver.Service

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
	hookResolver *hooks.Resolver
	hookTrust    HookTrustStore

	// recipesGlobalDir is the global recipes directory the workspace.recipes.list
	// discovery layers under a project's .lyra/recipes. Empty → project-only.
	recipesGlobalDir string

	// schedules is the scheduled-run registry (schedules.* + the scheduler
	// worker). nil when scheduling is unconfigured.
	schedules schedule.Service

	// @codebase semantic index: embeddingCell holds the live embedding role,
	// embeddings builds+caches embedders from it, embeddingStore persists it, and
	// codebaseIndex is the index service (nil when no CodebaseStore). See
	// embedding.go.
	embeddingCell  *atomic.Pointer[embeddingRole]
	embeddings     *embeddingResolver
	embeddingStore EmbeddingRoleStore
	codebaseIndex  codebaseindex.Service

	// transactor runs a write-set inside one storage transaction so the
	// cross-store operations (sessions.import / rollback) are atomic; nil → run
	// directly (RunInTx). See [Transactor].
	transactor Transactor
}

// Transactor runs fn inside a single storage transaction — the seam the
// composition root uses to give the Runtime cross-store atomicity without
// coupling it to the sqlite backend.
type Transactor func(ctx context.Context, fn func(context.Context) error) error

// RunInTx runs fn inside one storage transaction (commit on success, rollback
// on error), so a multi-step write-set across the domain services commits
// atomically. Falls back to running fn directly when no transactor is wired (a
// non-sqlite / test runtime) — correct but without all-or-nothing.
func (r *Runtime) RunInTx(ctx context.Context, fn func(context.Context) error) error {
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
	ecfg.SessionStore = newChildSessionStore(cfg.SessionService)
	// The default provider id — the engine's pricing fallback for a default /
	// subtask turn that names no provider (so its cost attributes to the right
	// provider rather than an alphabetical catalog guess).
	ecfg.Provider = cfg.Provider

	// Microkernel port wiring: the runtime is the composition root that builds
	// the capability implementations and injects them into the engine core
	// (which depends only on the port interfaces). All share one chat-memory
	// store so the engine's chat-memory middleware and these ports agree.
	memStore := cfg.Engine.MemoryStore
	if memStore == nil {
		memStore = memory.NewInMemoryStore()
		ecfg.MemoryStore = memStore
	}
	// conv is the message-history service. The engine gets it ONLY as the
	// turn-end steering sink (engine.InjectUserMessage); the runtime holds it
	// directly for the non-turn history operations (read/seed/count/truncate,
	// for fork / rollback / messages.list) rather than proxying them through the
	// engine. See doc/GREENFIELD_ARCHITECTURE.md.
	conv := conversation.New(memStore)

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
	providerSvc := cfg.ProviderService
	resolver := newClientResolver(providerSvc)

	// Utility-model role: the (provider, model) the in-house maintenance
	// services run on, loaded from its persistent store into an atomic cell so
	// models.setUtilityRole can repoint it live. resolveUtility reads the cell
	// per call (re-read, never captured) and resolves the client, falling back
	// to the main turn client when the role is unset or unresolvable — those
	// services degrade to the main model rather than failing.
	var role utilityRole
	if cfg.UtilityRoleStore != nil {
		p, m, lerr := cfg.UtilityRoleStore.LoadUtilityRole(ctx)
		if lerr != nil {
			return nil, fmt.Errorf("runtime: load utility role: %w", lerr)
		}
		role = utilityRole{provider: p, model: m}
	}
	utilCell := &atomic.Pointer[utilityRole]{}
	utilCell.Store(&role)
	mainClient := cfg.Engine.ChatClient
	resolveUtility := func(ctx context.Context) *chat.Client {
		role := utilCell.Load()
		if role == nil || role.model == "" {
			return mainClient
		}
		c, rerr := resolver.ResolveClient(ctx, role.provider, role.model)
		if rerr != nil || c == nil {
			return mainClient
		}
		return c
	}

	// Embedding-model role + the @codebase semantic index. The role (provider,
	// model) is loaded into an atomic cell so models.setEmbeddingRole repoints it
	// live; resolveEmbedder reads the cell per call and builds the embedder from
	// the provider registry, returning ErrNoEmbeddingModel when unset (the index
	// feature is then off). The index service is built only when a store is wired.
	embeddings := newEmbeddingResolver(providerSvc)
	embCell := &atomic.Pointer[embeddingRole]{}
	var erole embeddingRole
	if cfg.EmbeddingRoleStore != nil {
		p, m, lerr := cfg.EmbeddingRoleStore.LoadEmbeddingRole(ctx)
		if lerr != nil {
			return nil, fmt.Errorf("runtime: load embedding role: %w", lerr)
		}
		erole = embeddingRole{provider: p, model: m}
	}
	embCell.Store(&erole)
	resolveEmbedder := func(ctx context.Context) (codebaseindex.Embedder, error) {
		role := embCell.Load()
		if role == nil || role.model == "" {
			return nil, codebaseindex.ErrNoEmbeddingModel
		}
		return embeddings.resolve(ctx, role.provider, role.model)
	}
	var codebaseIdx codebaseindex.Service
	if cfg.CodebaseStore != nil {
		codebaseIdx = codebaseindex.New(cfg.CodebaseStore, resolveEmbedder)
	}

	if ecfg.Steering == nil {
		ecfg.Steering = conv
	}
	if ecfg.Compactor == nil {
		// Window-relative compaction trigger: resolve the default turn model's
		// context window from the catalog so compaction fires relative to the
		// real model (not a fixed 100k that's wrong for a 32k or a 1M window).
		// Catalog miss → ContextWindow 0 → the compactor's fixed fallback. Uses
		// the DEFAULT model's window; a turn that picks a smaller per-run model
		// keeps the default's headroom (documented limitation — compaction also
		// runs on the default utility client, so it stays self-consistent).
		window := 0
		if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
			window = int(info.Limits.ContextWindow)
		}
		ecfg.Compactor = maintenance.NewCompactor(memStore, resolveUtility, maintenance.CompactionConfig{ContextWindow: window})
	}
	if ecfg.Extractor == nil && cfg.Engine.Knowledge != nil {
		ecfg.Extractor = maintenance.NewExtractor(memStore, cfg.Engine.Knowledge, resolveUtility)
	}
	// Todo list: same nil-default contract — honor a pre-injected engine
	// Todos (an external task store), else use the runtime-supplied one.
	if ecfg.Todos == nil {
		ecfg.Todos = cfg.TodoService
	}

	// Tool environment: assembled outside the core (constructs the code-intel /
	// exec / MCP / A2A capabilities + the resolver) and injected, so the engine
	// core builds no capability. ctx flows so a slow MCP/A2A dial can be
	// canceled during startup.
	// Approval stance is built early: the toolset's exit_plan_mode tool needs it
	// (it flips the stance to execute when a plan is approved), and the turn gate
	// reads it per tool call.
	approvalSvc := approval.New(cfg.ApprovalMode, cfg.ApprovalRuleStore)

	// Per-call MCP tool gating, derived from the registry. The cell is created
	// up front so the two reader closures below — the resolver's disabled filter
	// and the turn gate's auto-approve skip — close over the SAME atomic the
	// Runtime later swaps on every registry change. A read failure is fatal: the
	// gating is part of the tool environment's contract.
	mcpGate := &atomic.Pointer[mcpGating]{}
	g0, err := buildMCPGating(ctx, cfg.MCPRegistry)
	if err != nil {
		return nil, fmt.Errorf("runtime: load mcp gating: %w", err)
	}
	mcpGate.Store(g0)
	mcpDisabled := func() map[string]struct{} {
		if g := mcpGate.Load(); g != nil {
			return g.disabled
		}
		return nil
	}
	mcpAutoApprove := func() map[string]struct{} {
		if g := mcpGate.Load(); g != nil {
			return g.autoApprove
		}
		return nil
	}

	// Boot MCP set = the registry's enabled servers (the env seed already
	// landed there in the composition root). A registry read failure is fatal —
	// MCP is part of the tool environment.
	mcpConfigs, err := enabledConfigs(ctx, cfg.MCPRegistry)
	if err != nil {
		return nil, fmt.Errorf("runtime: load mcp registry: %w", err)
	}

	built, err := toolset.Build(ctx, toolset.BuildConfig{
		Workdir:         cfg.Engine.Workdir,
		SkillsGlobalDir: cfg.Engine.SkillsGlobalDir,
		Online:          cfg.Online,
		LSPServers:      cfg.LSPServers,
		MCPServers:      mcpConfigs,
		A2AAgents:       cfg.A2AAgents,
		Todos:           ecfg.Todos,
		Approval:        approvalSvc,
		MCPDisabled:     mcpDisabled,
		CodebaseIndex:   codebaseIdx,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: build tools: %w", err)
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
	// composition root (cmd/lyra wires sqlite-backed services; tests wire a
	// sqlite :memory: DB). The runtime keeps no in-memory fallback — there's
	// a single storage backend now.
	sessionSvc := cfg.SessionService
	interruptStore := cfg.InterruptStore

	chatSvc, err := turn.New(eng, approvalSvc, resolver, ecfg.Todos, mcpAutoApprove, cfg.HooksResolver)
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
		chat:             chatSvc,
		session:          sessionSvc,
		tool:             toolSvc,
		knowledge:        cfg.Engine.Knowledge,
		approval:         approvalSvc,
		interrupts:       interruptStore,
		transcript:       cfg.TranscriptStore,
		conversation:     conv,
		providers:        providerSvc,
		mcpRegistry:      cfg.MCPRegistry,
		mcpGating:        mcpGate,
		defaultProvider:  cfg.Provider,
		defaultModel:     cfg.Model,
		titler:           maintenance.NewTitler(resolveUtility),
		utility:          utilCell,
		resolver:         resolver,
		utilStore:        cfg.UtilityRoleStore,
		hookResolver:     cfg.HooksResolver,
		hookTrust:        cfg.HookTrustStore,
		recipesGlobalDir: cfg.RecipesGlobalDir,
		schedules:        cfg.ScheduleStore,
		embeddingCell:    embCell,
		embeddings:       embeddings,
		embeddingStore:   cfg.EmbeddingRoleStore,
		codebaseIndex:    codebaseIdx,
		transactor:       cfg.Transactor,
	}, nil
}

// InspectHooks returns the lifecycle hooks discovered for cwd plus the project's
// trust status (workspace.hooks.list). Empty when hooks are unconfigured.
func (r *Runtime) InspectHooks(ctx context.Context, cwd string) hooks.Inspection {
	return r.hookResolver.Inspect(ctx, cwd)
}

// SetProjectHookTrust trusts (or revokes) a project's hooks (workspace.hooks.
// setTrust). No-op when no trust store is wired.
func (r *Runtime) SetProjectHookTrust(ctx context.Context, projectRoot string, trusted bool) error {
	if r.hookTrust == nil {
		return nil
	}
	if trusted {
		return r.hookTrust.Trust(ctx, projectRoot)
	}
	return r.hookTrust.Untrust(ctx, projectRoot)
}

// ListRecipes enumerates the prompt recipes visible from cwd — project recipes
// (<cwd>/.lyra/recipes) layered over the global directory, project winning on a
// name collision (workspace.recipes.list). The client expands a chosen recipe's
// body and sends it as a turn; the runtime only discovers them.
func (r *Runtime) ListRecipes(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	return recipes.List(ctx, recipes.ProjectDir(cwd), r.recipesGlobalDir)
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

// Chat returns the ChatService — the one-turn dispatch surface
// transport adapters call into for [turn.Service.StartTurn] etc.
func (r *Runtime) Chat() turn.Service { return r.chat }

// Session returns the SessionService — CRUD over saved sessions.
func (r *Runtime) Session() sessionsvc.Service { return r.session }

// Tool returns the ToolService — metadata + manual invocation surface.
func (r *Runtime) Tool() toolsvc.Service { return r.tool }

// Memory returns the LYRA.md cascade service — the wire/API "memory"
// surface (memory.get/update/list). Nil when no knowledge service was
// configured. The accessor keeps the wire term; the field is "knowledge".
func (r *Runtime) Memory() knowledge.Service { return r.knowledge }

// Approval returns the ApprovalService. Always non-nil — the runtime
// constructs one regardless of cfg.ApprovalMode (defaults to YOLO).
func (r *Runtime) Approval() approval.Service { return r.approval }

// Schedules returns the scheduled-run registry (schedules.* + the scheduler
// worker). nil when scheduling is unconfigured.
func (r *Runtime) Schedules() schedule.Service { return r.schedules }

// Interrupts returns the open-interrupt registry (R-model HITL resume
// discovery). Always non-nil.
func (r *Runtime) Interrupts() interrupts.Store { return r.interrupts }

// Transcript returns the durable Item-history store items.list is served
// from. Always non-nil — TranscriptStore is a required dependency.
func (r *Runtime) Transcript() transcript.Store { return r.transcript }

// MCPServerStatuses returns the per-server connection state of every
// configured MCP server (connected and boot-failed alike) for
// workspace.mcp.listServers. Delegates to the engine, which owns the sessions.
func (r *Runtime) MCPServerStatuses() []kernel.McpServerStatus {
	return r.engine.MCPServerStatuses()
}

// ReconnectMCPServer re-dials a configured MCP server and hot-swaps the live
// tool set (workspace.mcp.reconnect). Delegates to the engine, which owns the
// sessions + the shared client.
func (r *Runtime) ReconnectMCPServer(ctx context.Context, name string) error {
	return r.engine.ReconnectMCPServer(ctx, name)
}

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP MCP server
// (workspace.mcp.authorize) — opens the system browser, catches the loopback
// redirect, and connects on success. Delegates to the engine, which owns the
// sessions. The credentials live for the process only (re-prompt after restart).
func (r *Runtime) AuthorizeMCPServer(ctx context.Context, name string) error {
	return r.engine.AuthorizeMCPServer(ctx, name)
}

// Providers returns the provider registry — the runtime-mutable set of
// providers + credentials that providers.list / configure / test operate on.
// Always non-nil.
func (r *Runtime) Providers() provider.Service { return r.providers }

// ProbeProvider validates a provider's credentials by building its
// default-model client and issuing one minimal (max_tokens=1) request — the
// cheapest call that proves the key + endpoint work. Backs providers.test.
// Lives here, not in the protocol layer, because the runtime owns client
// construction. Returns the provider error verbatim so the caller can surface
// it inline.
func (r *Runtime) ProbeProvider(ctx context.Context, entry provider.Provider) error {
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(entry.ID),
		Model:    llm.DefaultModel(llm.Provider(entry.ID)),
		APIKey:   entry.APIKey,
		BaseURL:  entry.BaseURL,
	})
	if err != nil {
		return err
	}
	maxTokens := int64(1)
	_, err = client.Chat().
		WithOptions(&chat.Options{MaxTokens: &maxTokens}).
		WithUserPrompt("ping").
		Call().
		Response(ctx)
	return err
}

// DefaultModel is the model a turn runs against when it doesn't pick one
// (the configured Config.Model seed). The session layer uses it to fill
// Session.model for sessions that never explicitly selected a model, so the
// wire always carries a real model name. May be empty if unconfigured.
func (r *Runtime) DefaultModel() string { return r.defaultModel }

// DefaultProvider is the provider a turn runs against when a run names none
// (paired with DefaultModel). usage.summary uses it to attribute default-model
// runs (whose RunRef carries no provider) to the real provider. May be empty.
func (r *Runtime) DefaultProvider() string { return r.defaultProvider }

// GenerateTitle derives a short session title from a conversation's opening
// user message — auto-naming an untitled session (the wire Session.title).
// Best-effort: returns "" (no error) when titling isn't possible. Lives here,
// like [Runtime.ProbeProvider], because the runtime owns the maintenance LLM
// client; the delivery layer triggers it off a finished root run.
func (r *Runtime) GenerateTitle(ctx context.Context, firstMessage string) (string, error) {
	return r.titler.Generate(ctx, firstMessage)
}

// ReadHistory returns sessionID's persisted chat history — the
// messages.list transport surface converts these to wire messages,
// and ForkSession copies a prefix of them.
func (r *Runtime) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return r.conversation.Read(ctx, sessionID)
}

// SeedHistory copies msgs into sessionID's chat history — used by
// ForkSession to seed a fresh child with the parent's prefix.
func (r *Runtime) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return r.conversation.Seed(ctx, sessionID, msgs)
}

// MessageCount returns sessionID's chat-memory message count — the per-run
// watermark sessions.rollback / fork record.
func (r *Runtime) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return r.conversation.Count(ctx, sessionID)
}

// TruncateMessages keeps the first keepN chat-memory messages of sessionID
// (sessions.rollback).
func (r *Runtime) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	return r.conversation.Truncate(ctx, sessionID, keepN)
}

// ListSkills enumerates the skills visible from cwd (project over global) for
// workspace.listSkills. Delegates to the engine, which owns skill sourcing.
func (r *Runtime) ListSkills(ctx context.Context, cwd string) ([]kernel.SkillInfo, error) {
	return r.engine.ListSkills(ctx, cwd)
}

// MCPTools lists tools advertised by the connected MCP servers (scoped to
// server when non-empty) for workspace.mcp.listTools. Delegates to the
// engine, which holds the dialed sessions.
func (r *Runtime) MCPTools(ctx context.Context, server string) ([]kernel.McpToolInfo, error) {
	return r.engine.MCPTools(ctx, server)
}

// Close releases per-runtime external resources — MCP sessions and
// any future engine-owned handles. Idempotent.
func (r *Runtime) Close() error {
	if r == nil || r.engine == nil {
		return nil
	}
	return r.engine.Close()
}
