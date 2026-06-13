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

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/domain/approval"
	"github.com/Tangerg/lynx/lyra/internal/domain/codeintel"
	"github.com/Tangerg/lynx/lyra/internal/domain/conversation"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/domain/knowledge"
	"github.com/Tangerg/lynx/lyra/internal/domain/maintenance"
	"github.com/Tangerg/lynx/lyra/internal/domain/provider"
	sessionsvc "github.com/Tangerg/lynx/lyra/internal/domain/session"
	"github.com/Tangerg/lynx/lyra/internal/domain/todo"
	toolsvc "github.com/Tangerg/lynx/lyra/internal/domain/tool"
	"github.com/Tangerg/lynx/lyra/internal/domain/transcript"
	"github.com/Tangerg/lynx/lyra/internal/infra/a2a"
	"github.com/Tangerg/lynx/lyra/internal/infra/mcp"
	"github.com/Tangerg/lynx/lyra/internal/kernel"
	chatsvc "github.com/Tangerg/lynx/lyra/internal/kernel/chat"
	"github.com/Tangerg/lynx/lyra/internal/kernel/toolset"
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

	// MaintenanceClient optionally runs the in-house turn-boundary maintenance
	// services (compaction / extraction / planning) on a separate — typically
	// cheaper — model than Engine.ChatClient. nil runs them on the main client.
	// Only applies to the runtime's own maintenance services; an externally
	// injected Compactor/Extractor/Planner brings its own client.
	MaintenanceClient *chat.Client

	// Tool-environment inputs — the runtime reads these to assemble the tool
	// environment via toolset.Build and inject it into the engine core (which
	// constructs no capability itself). Workdir / SkillsGlobalDir come from
	// Engine (the engine also needs them for the prompt cascade / listSkills).
	Online     kernel.OnlineConfig    // network-tool credentials
	MCPServers []mcp.ServerConfig     // external MCP servers to dial
	A2AAgents  []a2a.ClientConfig     // remote A2A agents to dial
	LSPServers []codeintel.ServerSpec // language-server table (nil → defaults)

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

	// Provider / Model name the runtime's DEFAULT provider+model — the one a
	// turn runs against when it doesn't pick a model. providers.list /
	// models.list are served from the registry + catalog, not these.
	Provider string
	Model    string
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
	chat       chatsvc.Service
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

	providers    provider.Service
	defaultModel string
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
	// engine. See doc/STRUCTURE_REVIEW.md §3.
	conv := conversation.New(memStore)

	// Capability ports are SPIs: the engine consumes interfaces (Steering /
	// Compactor / Extractor / Planner; Knowledge above). The runtime supplies the
	// in-house implementations ONLY when the composition root didn't inject one,
	// so an external provider (e.g. a mem0 / HTTP-bridged compactor or knowledge
	// store) can be slotted in by setting the corresponding engine.Config field —
	// the runtime then leaves it untouched. nil → in-house default.
	// Maintenance (compaction / extraction / planning) may run on a cheaper
	// model than the main turn — see [Config.MaintenanceClient]. nil falls
	// back to the engine's client, preserving the single-model default. Only
	// the in-house services below use it; an externally injected port brings
	// its own client.
	maintClient := cfg.MaintenanceClient
	if maintClient == nil {
		maintClient = cfg.Engine.ChatClient
	}

	if ecfg.Steering == nil {
		ecfg.Steering = conv
	}
	if ecfg.Compactor == nil {
		ecfg.Compactor = maintenance.NewCompactor(memStore, maintClient, maintenance.CompactionConfig{})
	}
	if ecfg.Planner == nil {
		ecfg.Planner = maintenance.NewPlanner(maintClient)
	}
	if ecfg.Extractor == nil && cfg.Engine.Knowledge != nil {
		ecfg.Extractor = maintenance.NewExtractor(memStore, cfg.Engine.Knowledge, maintClient)
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
	built, err := toolset.Build(ctx, toolset.BuildConfig{
		Workdir:         cfg.Engine.Workdir,
		SkillsGlobalDir: cfg.Engine.SkillsGlobalDir,
		Online:          cfg.Online,
		LSPServers:      cfg.LSPServers,
		MCPServers:      cfg.MCPServers,
		A2AAgents:       cfg.A2AAgents,
		Todos:           ecfg.Todos,
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

	chatSvc, err := chatsvc.New(eng, approvalSvc, resolver)
	if err != nil {
		return nil, fmt.Errorf("runtime: chat service: %w", err)
	}
	toolSvc, err := toolsvc.New(eng)
	if err != nil {
		return nil, fmt.Errorf("runtime: tool service: %w", err)
	}

	return &Runtime{
		engine:       eng,
		chat:         chatSvc,
		session:      sessionSvc,
		tool:         toolSvc,
		knowledge:    cfg.Engine.Knowledge,
		approval:     approvalSvc,
		interrupts:   interruptStore,
		transcript:   cfg.TranscriptStore,
		conversation: conv,
		providers:    providerSvc,
		defaultModel: cfg.Model,
	}, nil
}

// Chat returns the ChatService — the one-turn dispatch surface
// transport adapters call into for [chatsvc.Service.StartTurn] etc.
func (r *Runtime) Chat() chatsvc.Service { return r.chat }

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
