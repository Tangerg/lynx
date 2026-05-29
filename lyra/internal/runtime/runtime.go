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
	chatmem "github.com/Tangerg/lynx/core/model/chat/memory"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	chatsvc "github.com/Tangerg/lynx/lyra/internal/service/chat"
	memsvc "github.com/Tangerg/lynx/lyra/internal/service/memory"
	sessionsvc "github.com/Tangerg/lynx/lyra/internal/service/session"
	toolsvc "github.com/Tangerg/lynx/lyra/internal/service/tool"
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
	MCPServers []engine.MCPServer

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

	// SessionService persists Lyra sessions. nil falls back to
	// [sessionsvc.NewInMemoryService] — same restart caveat.
	SessionService sessionsvc.Service

	// ApprovalMode sets the initial runtime approval stance. The
	// service is always constructed; mode defaults to [approval.ModeYolo]
	// when this field is the zero value.
	ApprovalMode approval.Mode
}

// Runtime is the bundle. Construct once via [New]; share the
// pointer across every transport adapter that needs to dispatch
// turns / sessions / approvals.
//
// Concurrency: every accessor returns a Service whose own methods
// are safe for concurrent use. Runtime itself holds no mutable
// state after construction.
type Runtime struct {
	engine   *engine.Engine
	chat     chatsvc.Service
	session  sessionsvc.Service
	tool     toolsvc.Service
	memory   memsvc.Service
	approval approval.Service
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
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: engine: %w", err)
	}

	approvalSvc := approval.New(cfg.ApprovalMode)
	sessionSvc := cfg.SessionService
	if sessionSvc == nil {
		sessionSvc = sessionsvc.NewInMemoryService()
	}

	return &Runtime{
		engine:   eng,
		chat:     chatsvc.New(eng, approvalSvc),
		session:  sessionSvc,
		tool:     toolsvc.New(eng),
		memory:   cfg.MemoryService,
		approval: approvalSvc,
	}, nil
}

// Engine exposes the underlying engine for service implementations
// that need fine-grained control (most don't — prefer the typed
// service accessors). M-future: may go away when MCP / tool
// orchestration moves entirely behind chat.Service.
func (r *Runtime) Engine() *engine.Engine { return r.engine }

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
