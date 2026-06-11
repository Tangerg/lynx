package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
	"github.com/Tangerg/lynx/lyra/internal/engine/toolset"
	"github.com/Tangerg/lynx/lyra/internal/service/knowledge"
)

// Engine is the microkernel core: it drives the agent loop and depends on
// injected ports for the capabilities it consumes (doc/MICROKERNEL.md). It
// composes three concerns:
//
//   - chat execution: platform + agent drive [Engine.StartChat]
//     (async; returns a [ChatProcess] handle backed by a real
//     [runtime.AgentProcess]) and [Engine.RunChat] (sync wrapper) —
//     see chatturn.go / chatprocess.go
//   - maintenance:    the injected Compactor / Extractor / Planner ports
//     power [Engine.MaybeCompact] / [Engine.MaybeExtract] / plan mode
//   - context:        the Conversation port / memSvc / workdir feed the
//     system prompt and the chat-memory middleware
//
// The tool environment (resolver + tools + MCP facade + closers) is assembled
// outside the core by [toolset.Build] and injected via [Config]; the core
// constructs no capability. The chat service's own (unexported) engine
// interface narrows this surface to exactly the operations it needs.
type Engine struct {
	// Chat execution.
	platform *runtime.Platform
	agent    *core.Agent

	// Context inputs (read at SystemPrompt + chat-memory time).
	tools           []chat.Tool
	conversation    Conversation // LLM message history (fork/rollback/steering/messages.list facade)
	memSvc          knowledge.Service
	workdir         string  // captured from Config.Workdir for the AGENTS.md cascade
	skillsGlobalDir string  // captured from Config.SkillsGlobalDir for workspace.listSkills
	pricing         Pricing // optional per-round cost hook; nil → cost stays zero
	parkStore       tool.ParkStore

	// Maintenance ports (turn-boundary autonomous ops) — injected by the
	// composition root; nil when not wired (every use is nil-guarded).
	compactor Compactor
	extractor Extractor
	planner   Planner

	// mcp is the live-MCP-connections facade port (workspace.mcp.* views +
	// reconnect), assembled in the toolset layer and injected; nil when no MCP
	// servers are wired. closers are the capability shutdown hooks (code-intel
	// servers, background processes, MCP/A2A sessions) the toolset assembly
	// handed over, run in [Engine.Close].
	mcp     toolset.MCPControl
	closers []func() error
}

// New constructs an engine. Returns an error when required deps
// are missing or when agent deployment fails.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if cfg.ChatClient == nil {
		return nil, errors.New("engine: ChatClient is required")
	}

	// The tool environment (capability adapters + per-role/per-cwd resolver +
	// canonical tool list) is assembled OUTSIDE the core, in the toolset layer,
	// and injected via [Config.ToolResolver] / [Config.Tools] / [Config.MCP] /
	// [Config.Closers] (the composition root calls [toolset.Build]). The engine
	// core therefore constructs no capability and imports no infra/service for
	// them — it only drives the resolver + appends the two engine-owned tools
	// (task / ask_user) below.
	resolver := cfg.ToolResolver
	if resolver == nil {
		// A bare engine (unit tests that drive only the loop) gets an empty
		// resolver — no tools, but the loop still runs.
		resolver = toolset.NewResolver(toolset.Deps{DefaultWorkdir: cfg.Workdir, SkillsGlobalDir: cfg.SkillsGlobalDir})
	}

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
		conversation:    cfg.Conversation,
		compactor:       cfg.Compactor,
		extractor:       cfg.Extractor,
		planner:         cfg.Planner,
		memSvc:          cfg.MemoryService,
		workdir:         cfg.Workdir,
		skillsGlobalDir: cfg.SkillsGlobalDir,
		pricing:         cfg.Pricing,
		parkStore:       cfg.ParkStore,
		mcp:             cfg.MCP,
		closers:         cfg.Closers,
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
	resolver.SetTask(taskTool)

	// ask_user (HITL question) rides the engine's interrupt contract, so the
	// engine builds it and injects it into the resolver (coding role only).
	askUser := newAskUserTool()
	resolver.SetAskUser(askUser)

	// e.tools is the canonical coding tool set for ToolService.List — the
	// toolset-assembled list (working-directory-independent metadata) plus the
	// two engine-owned tools.
	e.tools = append(append([]chat.Tool{}, cfg.Tools...), taskTool, askUser)

	e.agent = e.buildChatAgent()
	if err := platform.Deploy(e.agent); err != nil {
		return nil, fmt.Errorf("engine: deploy chat agent: %w", err)
	}
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
func (e *Engine) MaybeCompact(ctx context.Context, sessionID string) (CompactionResult, error) {
	if e.compactor == nil {
		return CompactionResult{}, nil
	}
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
func (e *Engine) MaybeExtract(ctx context.Context, sessionID, cwd string) (ExtractionResult, error) {
	if e.extractor == nil {
		return ExtractionResult{}, nil
	}
	return e.extractor.MaybeExtract(ctx, sessionID, cwd)
}

// Tools returns the registered coding tool set — used by
// ToolService.List to surface tool metadata to clients without
// re-running the construction.
func (e *Engine) Tools() []chat.Tool { return e.tools }

// Close releases the per-engine external resources the toolset assembly opened
// (code-intelligence servers, background processes, MCP / A2A sessions) by
// running the capability closers it handed over. Safe to call multiple times; a
// nil/empty closer slice makes the second call a no-op.
//
// Errors from individual closures are collected and returned together so the
// caller can log them; partial failure does not stop subsequent closes.
func (e *Engine) Close() error {
	var errs []error
	for _, closeFn := range e.closers {
		if err := closeFn(); err != nil {
			errs = append(errs, err)
		}
	}
	e.closers = nil
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
	if e.conversation == nil {
		return errors.New("engine: no conversation port wired")
	}
	return e.conversation.InjectUser(ctx, sessionID, text)
}

// ReadHistory returns sessionID's persisted message history (messages.list /
// fork read it).
func (e *Engine) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	if e.conversation == nil {
		return nil, nil
	}
	return e.conversation.Read(ctx, sessionID)
}

// SeedHistory copies msgs into sessionID's history (sessions.fork).
func (e *Engine) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	if e.conversation == nil {
		return nil
	}
	return e.conversation.Seed(ctx, sessionID, msgs)
}

// MessageCount returns sessionID's message count (rollback / fork watermark).
func (e *Engine) MessageCount(ctx context.Context, sessionID string) (int, error) {
	if e.conversation == nil {
		return 0, nil
	}
	return e.conversation.Count(ctx, sessionID)
}

// TruncateMessages keeps the first keepN messages of sessionID (rollback).
func (e *Engine) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	if e.conversation == nil {
		return nil
	}
	return e.conversation.Truncate(ctx, sessionID, keepN)
}
