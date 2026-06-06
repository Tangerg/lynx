// Package runtime is Lyra's core-runtime façade — one struct that
// bundles the engine + every Service interface a transport adapter
// might need. The architecture goal documented in ARCHITECTURE.md is
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
//	    engine + service/*  (in-process implementations)
//
// Today the runtime + all transports live in the same Go process. The
// boundary still matters: transports depend on runtime, not on the
// concrete service constructors, so a future "remote" runtime impl
// (one process for the engine, another for the transport) only needs
// to satisfy [Runtime]'s accessor surface.
package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	chatmem "github.com/Tangerg/lynx/core/model/chat/memory"

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	chatsvc "github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
	memsvc "github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/lyra/internal/service/provider"
	sessionsvc "github.com/Tangerg/lynx/lyra/internal/service/session"
	toolsvc "github.com/Tangerg/lynx/lyra/internal/service/tool"
	"github.com/Tangerg/lynx/mcp"
)

// Config is the construction-time bundle for [New]. ChatClient is
// the only strictly required field — every other dependency has a
// sensible in-memory default for tests / smoke runs.
type Config struct {
	// ChatClient is the LLM client every action eventually calls
	// through to. Required.
	ChatClient *chat.Client

	// Workdir scopes filesystem-touching tools (fs / bash). Empty
	// disables scoping — fine for tests, NOT recommended for
	// production where the model could read anywhere on disk.
	Workdir string

	// Online toggles the provider-backed online tools. Each field is
	// independent; empty credentials skip the corresponding tool.
	Online engine.OnlineConfig

	// MCPServers lists external MCP servers to dial at startup.
	// Their tools merge into the engine's tool set under the
	// configured Name prefix.
	MCPServers []mcp.ServerConfig

	// Compaction tunes the post-turn auto-compaction. Zero values
	// fall back to the package defaults; setting MaxMessages
	// negative disables compaction entirely.
	Compaction engine.CompactionConfig

	// MemoryStore is the chat-memory backend. nil falls back to the
	// in-process [chatmem.InMemoryStore] (history lost on restart).
	MemoryStore chatmem.Store

	// MemoryService backs the LYRA.md cascade reader. nil disables
	// the cascade — the base system prompt is used verbatim.
	MemoryService memsvc.Service

	// SessionService persists Lyra sessions. Required — the composition
	// root injects the sqlite-backed service (tests use a sqlite :memory: DB).
	SessionService sessionsvc.Service

	// InterruptStore records open HITL interrupts (R-model resume
	// discovery). Required — injected sqlite-backed, same as SessionService.
	InterruptStore interrupts.Store

	// ApprovalMode sets the initial runtime approval stance. The
	// service is always constructed; mode defaults to [approval.ModeYolo]
	// when this field is the zero value.
	ApprovalMode approval.Mode

	// Pricing optionally computes per-round USD cost so turns report
	// CostUSD. nil leaves cost at zero. See [engine.Pricing].
	Pricing engine.Pricing

	// ProcessStore, when non-nil, persists agent-process snapshots
	// (audit + restart durability). nil = no persistence. See
	// [engine.Config.ProcessStore].
	ProcessStore core.ProcessStore

	// HistoryStore, when non-nil, persists the durable Item history that
	// items.list is served from (authoritative completed Items + their
	// RunRefs). nil falls back to deriving items from chat-memory messages.
	HistoryStore history.Store

	// Provider / Model name the runtime's DEFAULT provider+model — the one a
	// turn runs against when it doesn't pick a model. providers.list /
	// models.list are served from the registry + catalog, not these.
	Provider string
	Model    string

	// ProviderService is the runtime-mutable provider registry (per-provider
	// credentials, persisted). Required — the composition root injects the
	// sqlite-backed registry and seeds the configured provider into it.
	ProviderService provider.Service
}

// Runtime is the bundle. Construct once via [New]; share the
// pointer across every transport adapter that needs to dispatch
// turns / sessions / approvals.
//
// Concurrency: every accessor returns a Service whose own methods
// are safe for concurrent use. Runtime itself holds no mutable
// state after construction.
type Runtime struct {
	engine     *engine.Engine
	chat       chatsvc.Service
	session    sessionsvc.Service
	tool       toolsvc.Service
	memory     memsvc.Service
	approval   approval.Service
	interrupts interrupts.Store
	history    history.Store

	providers      provider.Service
	mcpServerNames []string
	defaultModel   string
}

// New assembles a Runtime from cfg. Returns an error when a required
// dependency (ChatClient) is missing or any internal constructor
// fails — engine deployment, MCP dial, etc.
func New(ctx context.Context, cfg Config) (*Runtime, error) {
	if cfg.ChatClient == nil {
		return nil, errors.New("runtime: ChatClient is required")
	}

	eng, err := engine.New(ctx, engine.Config{
		ChatClient:    cfg.ChatClient,
		Workdir:       cfg.Workdir,
		Online:        cfg.Online,
		MCPServers:    cfg.MCPServers,
		MemoryStore:   cfg.MemoryStore,
		MemoryService: cfg.MemoryService,
		Compaction:    cfg.Compaction,
		Pricing:       cfg.Pricing,
		ProcessStore:  cfg.ProcessStore,
		// When a sub-agent (the `task` delegation) is spawned, the runtime
		// records its session here so the parent→child lineage is durably
		// queryable; CreateSubtask marks it internal so it stays out of List.
		SessionStore: newChildSessionStore(cfg.SessionService),
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: engine: %w", err)
	}

	// session / interrupt / provider are required and injected by the
	// composition root (cmd/lyra wires sqlite-backed services; tests wire a
	// sqlite :memory: DB). The runtime keeps no in-memory fallback — there's
	// a single storage backend now.
	approvalSvc := approval.New(cfg.ApprovalMode)
	sessionSvc := cfg.SessionService
	interruptStore := cfg.InterruptStore
	providerSvc := cfg.ProviderService

	// The resolver lets a turn pick its model: given an explicit
	// (provider, model) it builds the client from that provider's registry
	// credentials. A turn with no selection runs the engine's default client.
	resolver := newClientResolver(providerSvc)

	return &Runtime{
		engine:         eng,
		chat:           chatsvc.New(eng, approvalSvc, resolver),
		session:        sessionSvc,
		tool:           toolsvc.New(eng),
		memory:         cfg.MemoryService,
		approval:       approvalSvc,
		interrupts:     interruptStore,
		history:        cfg.HistoryStore,
		providers:      providerSvc,
		mcpServerNames: mcpNamesFrom(cfg.MCPServers),
		defaultModel:   cfg.Model,
	}, nil
}

// mcpNamesFrom lifts the configured MCP server names. The runtime only
// starts when every server dialed successfully (engine construction fails
// otherwise), so a name present here is a connected server.
func mcpNamesFrom(servers []mcp.ServerConfig) []string {
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		out = append(out, s.Name)
	}
	return out
}

// Chat returns the ChatService — the one-turn dispatch surface
// transport adapters call into for [chatsvc.Service.StartTurn] etc.
func (r *Runtime) Chat() chatsvc.Service { return r.chat }

// Session returns the SessionService — CRUD over saved sessions.
func (r *Runtime) Session() sessionsvc.Service { return r.session }

// Tool returns the ToolService — metadata + manual invocation surface.
func (r *Runtime) Tool() toolsvc.Service { return r.tool }

// Memory returns the LYRA.md cascade service. Nil when no memory
// service was configured (cfg.MemoryService was nil).
func (r *Runtime) Memory() memsvc.Service { return r.memory }

// Approval returns the ApprovalService. Always non-nil — the runtime
// constructs one regardless of cfg.ApprovalMode (defaults to YOLO).
func (r *Runtime) Approval() approval.Service { return r.approval }

// Interrupts returns the open-interrupt registry (R-model HITL resume
// discovery). Always non-nil.
func (r *Runtime) Interrupts() interrupts.Store { return r.interrupts }

// History returns the durable Item-history store, or nil when none was
// configured (the RPC server then derives items.list from chat-memory
// messages instead).
func (r *Runtime) History() history.Store { return r.history }

// MCPServerNames returns the names of the MCP servers dialed at startup
// (all connected — see mcpNamesFrom).
func (r *Runtime) MCPServerNames() []string { return r.mcpServerNames }

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
	client, _, err := config.BuildClient(config.ClientSpec{
		Provider: config.Provider(entry.ID),
		Model:    config.DefaultModel(config.Provider(entry.ID)),
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

// ReadHistory returns sessionID's persisted chat history — the
// messages.list transport surface converts these to wire messages,
// and ForkSession copies a prefix of them. Delegates to the engine,
// which owns the chat-memory store.
func (r *Runtime) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return r.engine.ReadHistory(ctx, sessionID)
}

// SeedHistory copies msgs into sessionID's chat history — used by
// ForkSession to seed a fresh child with the parent's prefix.
func (r *Runtime) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return r.engine.SeedHistory(ctx, sessionID, msgs)
}

// Close releases per-runtime external resources — MCP sessions and
// any future engine-owned handles. Idempotent.
func (r *Runtime) Close() error {
	if r == nil || r.engine == nil {
		return nil
	}
	return r.engine.Close()
}

// MaybeMaintain runs the post-turn compaction + extraction pair —
// mostly a passthrough so transport adapters don't reach into the
// engine directly. Returns (compacted, nil) so callers can chain
// follow-on work conditionally.
//
// Lives here (not on chat.Service) because the maintenance is
// platform-level housekeeping; chat.Service.runTurn already calls
// it after each successful turn, but the standalone form lets
// scripts trigger it after bulk imports.
func (r *Runtime) MaybeMaintain(ctx context.Context, sessionID string) (bool, error) {
	compaction, err := r.engine.MaybeCompact(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if compaction.Compacted {
		if _, err := r.engine.MaybeExtract(ctx, sessionID); err != nil {
			return true, err
		}
	}
	return compaction.Compacted, nil
}
