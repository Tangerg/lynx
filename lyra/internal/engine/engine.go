package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/a2a"
	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
	"github.com/Tangerg/lynx/lyra/internal/infra/exec"
	"github.com/Tangerg/lynx/lyra/internal/service/codeintel"
	"github.com/Tangerg/lynx/lyra/internal/service/conversation"
	"github.com/Tangerg/lynx/lyra/internal/service/knowledge"
	"github.com/Tangerg/lynx/lyra/internal/service/maintenance"
)

// Engine is the runtime facade. It composes three concerns:
//
//   - chat execution: platform + agent drive [Engine.StartChat]
//     (async; returns a [ChatProcess] handle backed by a real
//     [runtime.AgentProcess]) and [Engine.RunChat] (sync wrapper) —
//     see chatturn.go / chatprocess.go
//   - maintenance:    compactor / extractor / planner power
//     [Engine.MaybeCompact] / [Engine.MaybeExtract]
//   - context:        conversation / memSvc / workdir feed the
//     system prompt and the chat-memory middleware
//
// Each sub-component is a focused struct in its own file; Engine
// just owns construction and the public surface. The chat service's
// own (unexported) engine interface narrows this to exactly the
// operations that service needs.
type Engine struct {
	// Chat execution.
	platform *runtime.Platform
	agent    *core.Agent

	// Context inputs (read at SystemPrompt + chat-memory time).
	tools           []chat.Tool
	conversation    *conversation.Service // LLM message history (fork/rollback/steering/messages.list facade)
	memSvc          knowledge.Service
	workdir         string  // captured from Config.Workdir for the AGENTS.md cascade
	skillsGlobalDir string  // captured from Config.SkillsGlobalDir for workspace.listSkills
	pricing         Pricing // optional per-round cost hook; nil → cost stays zero
	parkStore       tool.ParkStore

	// Maintenance sub-components — each may be nil when the
	// corresponding feature is disabled by config (e.g. extractor
	// when no MemoryService was supplied).
	compactor *maintenance.Compactor
	extractor *maintenance.Extractor
	planner   *maintenance.Planner

	// External lifecycle. mcpServers (per-server session + status) and
	// a2aClients are closed during [Engine.Close]; nil when no MCP servers /
	// A2A agents are wired. mcpServers also backs the per-server
	// workspace.mcp.{listServers,listTools} views — including servers that
	// failed to connect at boot (recorded, not dropped).
	//
	// mcpMu guards every mcpServer's mutable fields (session/status/lastErr):
	// boot writes happen-before the engine is published, ReconnectMCPServer is
	// the only post-boot mutator, and MCPTools / MCPServerStatuses read under
	// it. mcpClient is the shared client reconnect re-dials with; toolResolver
	// is held so a reconnect can hot-swap the live MCP tool set.
	mcpMu        sync.Mutex
	mcpServers   []*mcpServer
	mcpClient    *sdkmcp.Client
	toolResolver *cwdToolResolver
	a2aClients   []*a2aclient.Client

	// codeIntel drives language servers (gopls, …) for the code-intelligence
	// tools and the post-edit diagnostics wrap. Servers launch lazily per
	// (workspace root, language) and are shut down in Close. nil only in a
	// never-constructed engine.
	codeIntel *codeintel.Service

	// bgShells owns the background-command processes; killed in Close.
	bgShells *exec.Manager
}

// New constructs an engine. Returns an error when required deps
// are missing or when agent deployment fails.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if cfg.ChatClient == nil {
		return nil, errors.New("engine: ChatClient is required")
	}

	// The online (network) and MCP tools are working-directory
	// independent, so they're built once and captured by the resolver.
	// The filesystem + bash tools are rebuilt per resolution against the
	// turn's working directory — see cwdToolResolver.
	online, err := buildOnlineTools(cfg.Online)
	if err != nil {
		return nil, fmt.Errorf("engine: build online tools: %w", err)
	}

	// One MCP client identity for every server — none of lyra's connections
	// need per-server client handlers (sampling / list-changed), so they share
	// it. Retained on the engine so ReconnectMCPServer can re-dial with it.
	mcpClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime", Version: "v0.1.0"}, nil)

	// Dial MCP servers so the model can call them transparently. The dial
	// happens before resolver wiring so the resolver captures the set in
	// one place. ctx flows from the caller so a slow / unreachable MCP
	// server can be canceled during startup. A single unreachable server is
	// tolerated (recorded "failed"); only a config mistake fails the call.
	mcpServers, mcpTools, err := dialMCPServers(ctx, mcpClient, cfg.MCPServers)
	if err != nil {
		return nil, err
	}

	// Dial remote A2A agents (one delegation tool each). On failure, close
	// the MCP sessions already opened above so a late startup error doesn't
	// leak them — Engine.Close never runs because New returns before e exists.
	a2aTools, a2aClients, err := dialA2AAgents(ctx, cfg.A2AAgents)
	if err != nil {
		for _, ms := range mcpServers {
			if ms.session != nil {
				_ = ms.session.Close()
			}
		}
		return nil, err
	}

	// One platform-scope resolver serves both roles. ToolRoleSubtask
	// resolves the cwd-bound coding tools + online + MCP + A2A; ToolRoleCoding
	// adds the `task` delegation tool, wired below once it exists (it
	// needs the platform). The resolver reads each turn's working
	// directory off the process blackboard at resolution time.
	// LSP code-intelligence: one service per engine wrapping the LSP manager,
	// servers launched lazily per (workspace root, language). The tools are
	// cwd-independent (the service keys by root, read per-call off the
	// blackboard) so they're built once here alongside online / A2A. An empty
	// server table falls back to the built-in defaults; config can replace it
	// wholesale.
	codeIntel := codeintel.New(cfg.LSPServers)
	lspTools := buildLSPTools(codeIntel, cfg.Workdir)

	// readTracker backs the read-before-edit + stale-file guards on the fs
	// read/edit/write tools (per-session, in-memory).
	tracker := newReadTracker()

	// Background-command tools (run_in_background / bash_output / kill_shell)
	// over one per-engine manager; processes are killed in Close.
	bgShells := exec.NewManager()
	bgShellTools := buildBgShellTools(bgShells, cfg.Workdir)

	resolver := &cwdToolResolver{
		defaultWorkdir:  cfg.Workdir,
		skillsGlobalDir: cfg.SkillsGlobalDir,
		online:          online,
		a2a:             a2aTools,
		lsp:             lspTools,
		codeIntel:       codeIntel,
		readTracker:     tracker,
		bgShell:         bgShellTools,
	}
	resolver.setMCPTools(mcpTools) // seed the hot-swappable MCP set (reconnect re-stores it)

	memStore := cfg.MemoryStore
	if memStore == nil {
		memStore = memory.NewInMemoryStore()
	}
	callMW, streamMW, err := memory.NewMiddleware(memStore)
	if err != nil {
		return nil, fmt.Errorf("engine: build memory middleware: %w", err)
	}
	// One tool + memory middleware chain serves every process — top-level
	// turns and spawned subtasks alike. Each request carries its own
	// conversation id (the agent runtime stamps [chat.ConversationIDKey]
	// from the process's Session), so a single shared chain keys memory and
	// park state per-process without per-turn reconstruction.
	toolCallMW, toolStreamMW := tool.NewMiddleware(tool.Config{
		FeedbackOnEmptyResponse: true,
		ParkStore:               cfg.ParkStore,
	})
	platform := agent.NewPlatform(runtime.PlatformConfig{
		ChatClient: cfg.ChatClient,
		Extensions: []core.Extension{resolver},
		Guardrails: &core.Guardrails{
			CallMiddlewares:   []chat.CallMiddleware{toolCallMW, callMW},
			StreamMiddlewares: []chat.StreamMiddleware{toolStreamMW, streamMW},
		},
		// Auto-snapshot only when a store is configured — no store, no
		// per-tick disk churn.
		ProcessStore: cfg.ProcessStore,
		AutoSnapshot: cfg.ProcessStore != nil,
		// Persist sub-agent (subtask) sessions so the delegation lineage is
		// durably recorded; the engine just forwards the store.
		SessionStore: cfg.SessionStore,
	})

	// Build the engine value first so the agent's Action closure can
	// capture *Engine (and therefore reach e.SystemPrompt) instead
	// of dragging a memory service through the constructor.
	e := &Engine{
		platform:        platform,
		conversation:    conversation.New(memStore),
		memSvc:          cfg.MemoryService,
		workdir:         cfg.Workdir,
		skillsGlobalDir: cfg.SkillsGlobalDir,
		pricing:         cfg.Pricing,
		parkStore:       cfg.ParkStore,
		mcpServers:      mcpServers,
		mcpClient:       mcpClient,
		toolResolver:    resolver,
		a2aClients:      a2aClients,
		codeIntel:       codeIntel,
		bgShells:        bgShells,
	}

	// The `task` tool delegates to a fresh sub-agent (declares
	// ToolRoleSubtask → no `task` → no recursion). Hand it to the
	// resolver, which folds it into the ToolRoleCoding set only.
	// AsChatToolFromAgent needs no separate deploy — child processes land
	// on the platform when spawned.
	taskTool, err := runtime.AsChatToolFromAgent[taskInput, string](platform, e.buildSubtaskAgent())
	if err != nil {
		return nil, fmt.Errorf("engine: build task tool: %w", err)
	}
	resolver.task = taskTool

	// e.tools is the canonical coding tool set for ToolService.List —
	// tool metadata (name / schema) is working-directory independent, so
	// the default-workdir build is a faithful representative of what any
	// turn resolves.
	e.tools = append(buildWorkdirTools(cfg.Workdir, codeIntel, tracker), online...)
	e.tools = append(e.tools, mcpTools...)
	e.tools = append(e.tools, a2aTools...)
	e.tools = append(e.tools, lspTools...)
	e.tools = append(e.tools, bgShellTools...)
	if skillTool := buildSkillTool(cfg.Workdir, cfg.SkillsGlobalDir); skillTool != nil {
		e.tools = append(e.tools, skillTool)
	}
	e.tools = append(e.tools, taskTool)
	e.tools = append(e.tools, newAskUserTool())

	e.agent = e.buildChatAgent()
	if err := platform.Deploy(e.agent); err != nil {
		return nil, fmt.Errorf("engine: deploy chat agent: %w", err)
	}

	if cfg.Compaction.MaxMessages >= 0 {
		e.compactor = maintenance.NewCompactor(memStore, cfg.ChatClient, cfg.Compaction)
	}
	if cfg.MemoryService != nil {
		e.extractor = maintenance.NewExtractor(memStore, cfg.MemoryService, cfg.ChatClient)
	}
	e.planner = maintenance.NewPlanner(cfg.ChatClient)
	return e, nil
}

// MaybeCompact runs one auto-compaction sweep against sessionID. The
// runtime calls this at every turn-end so growing histories get
// folded into a summary before the next turn starts. The returned
// [CompactionResult] reports whether the sweep fired (so callers can
// chain follow-on work like fact extraction) and the before/after
// message counts (so callers can surface an observable boundary).
//
// No-op (returns a zero CompactionResult) when:
//   - sessionID is empty (single-turn / no chat-memory path)
//   - the configured Compaction.MaxMessages is negative (disabled)
//   - the current history is shorter than the threshold
func (e *Engine) MaybeCompact(ctx context.Context, sessionID string) (maintenance.CompactionResult, error) {
	return e.compactor.MaybeCompact(ctx, sessionID)
}

// MaybeExtract mines the recent conversation for facts worth
// recording in <cwd>/LYRA.md. Best run right after MaybeCompact so
// the LLM sees a digest rather than a raw firehose. The returned
// [ExtractionResult] reports whether anything was written and the
// facts themselves, so callers can surface a memory-updated event.
//
// No-op (zero ExtractionResult) when the engine has no MemoryService
// or the conversation is too short.
// cwd is the session's working directory — facts extract into THAT
// project's LYRA.md; empty falls back to the memory service default.
func (e *Engine) MaybeExtract(ctx context.Context, sessionID, cwd string) (maintenance.ExtractionResult, error) {
	return e.extractor.MaybeExtract(ctx, sessionID, cwd)
}

// Tools returns the registered coding tool set — used by
// ToolService.List to surface tool metadata to clients without
// re-running the construction.
func (e *Engine) Tools() []chat.Tool { return e.tools }

// Close releases per-engine external resources — the MCP client sessions
// and remote A2A clients opened in [New]. Safe to call multiple times; nil
// slices make the second call a no-op.
//
// Errors from individual closures are collected and returned together so the
// caller can log them; partial failure does not stop subsequent closes.
func (e *Engine) Close() error {
	var errs []error
	for _, ms := range e.mcpServers {
		if ms.session == nil {
			continue
		}
		if err := ms.session.Close(); err != nil {
			errs = append(errs, err)
		}
		ms.session = nil
	}
	e.mcpServers = nil
	if err := a2a.CloseClients(e.a2aClients); err != nil {
		errs = append(errs, err)
	}
	e.a2aClients = nil
	if e.codeIntel != nil {
		if err := e.codeIntel.Close(); err != nil {
			errs = append(errs, err)
		}
		e.codeIntel = nil
	}
	if e.bgShells != nil {
		e.bgShells.KillAll()
		e.bgShells = nil
	}
	return errors.Join(errs...)
}

// The conversation surface (InjectUserMessage / ReadHistory / SeedHistory /
// MessageCount / TruncateMessages) is a thin facade over the conversation
// service — the engine exposes it for the chat service's steering seam and the
// runtime SPI (fork / rollback / messages.list); the domain logic lives in
// [conversation.Service].

// InjectUserMessage delivers mid-turn steering: chat.Service flushes a queued
// steering message through here once the current turn ends, so the next
// StartTurn (or post-turn maintenance) sees it as part of the conversation.
func (e *Engine) InjectUserMessage(ctx context.Context, sessionID, text string) error {
	return e.conversation.InjectUser(ctx, sessionID, text)
}

// ReadHistory returns sessionID's persisted message history (messages.list /
// fork read it).
func (e *Engine) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return e.conversation.Read(ctx, sessionID)
}

// SeedHistory copies msgs into sessionID's history (sessions.fork).
func (e *Engine) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return e.conversation.Seed(ctx, sessionID, msgs)
}

// MessageCount returns sessionID's message count (rollback / fork watermark).
func (e *Engine) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return e.conversation.Count(ctx, sessionID)
}

// TruncateMessages keeps the first keepN messages of sessionID (rollback).
func (e *Engine) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	return e.conversation.Truncate(ctx, sessionID, keepN)
}
