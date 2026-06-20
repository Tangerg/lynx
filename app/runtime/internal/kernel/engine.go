package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset"
)

// Engine is the microkernel core: it drives the agent loop and depends on
// injected ports for the capabilities it consumes (doc/GREENFIELD_ARCHITECTURE.md §5.1). It
// composes three concerns:
//
//   - chat execution: platform + agent drive [Engine.StartChat]
//     (async; returns a [ChatProcess] handle backed by a real
//     [runtime.AgentProcess]) and [Engine.RunChat] (sync wrapper) —
//     see chatturn.go / chatprocess.go
//   - maintenance:    the injected Compactor / Extractor ports power
//     [Engine.MaybeCompact] / [Engine.MaybeExtract]
//   - context:        knowledge / workdir feed the system prompt; the Steering
//     port flushes a queued steering message into history at turn-end
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
	steering        SteeringSink // turn-end steering inject; nil → steering drops
	knowledge       knowledge.Service
	todos           todo.Service // per-session task list; nil → todo_write absent + no prompt injection
	workdir         string       // captured from Config.Workdir for the AGENTS.md cascade
	skillsGlobalDir string       // captured from Config.SkillsGlobalDir for workspace.listSkills
	pricing         Pricing      // optional per-round cost hook; nil → cost stays zero
	parkStore       tool.ParkStore

	// Maintenance ports (turn-boundary autonomous ops) — injected by the
	// composition root; nil when not wired (every use is nil-guarded).
	compactor Compactor
	extractor Extractor

	// mcp is the live-MCP-connections facade port (workspace.mcp.* views +
	// reconnect), assembled in the toolset layer and injected; nil when no MCP
	// servers are wired. closers are the capability shutdown hooks (code-intel
	// servers, background processes, MCP/A2A sessions) the toolset assembly
	// handed over, run in [Engine.Close].
	mcp     toolset.MCPControl
	closers []func() error

	// closeOnce guards Close so concurrent / repeated calls run the closers
	// exactly once (a non-idempotent closer — e.g. closing an MCP session —
	// double-fired could panic). closeErr caches the joined result for callers
	// after the first.
	closeOnce sync.Once
	closeErr  error
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
		// Halt a stuck agent (same tool + args + result repeating) before it
		// burns the full iteration cap. Defaults (window 10, nudge 3, halt >5):
		// a corrective <system-reminder> is injected once at the 3rd identical
		// round (a chance to break out), and six byte-identical rounds is a fixed
		// point that hard-stops.
		LoopDetection: &tool.LoopDetectionConfig{},
		// Mid-run steering: drain the active turn's SteerSource (stashed on the
		// context by runChatTurn) before each continuation round, injecting any
		// queued user messages into the loop. nil source → no-op.
		BeforeRound: func(ctx context.Context) []chat.Message {
			if s := steerSourceFrom(ctx); s != nil {
				return s()
			}
			return nil
		},
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
		steering:        cfg.Steering,
		compactor:       cfg.Compactor,
		extractor:       cfg.Extractor,
		knowledge:       cfg.Knowledge,
		todos:           cfg.Todos,
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

	// e.tools is the canonical coding tool set for ToolService.List — the
	// toolset-assembled list (working-directory-independent metadata, which now
	// includes ask_user) plus the one engine-owned tool, `task` (it needs the
	// platform to spawn the sub-agent, so the engine builds + injects it).
	e.tools = append(append([]chat.Tool{}, cfg.Tools...), taskTool)

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
// No-op (zero ExtractionResult) when the engine has no knowledge service
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
// running the capability closers it handed over. Goroutine-safe and idempotent
// via sync.Once: concurrent or repeated calls run the closers exactly once and
// return the same joined result (running a closer twice could panic on a
// non-idempotent resource like an already-closed session).
//
// Errors from individual closures are collected and returned together so the
// caller can log them; partial failure does not stop subsequent closes.
func (e *Engine) Close() error {
	e.closeOnce.Do(func() {
		var errs []error
		for _, closeFn := range e.closers {
			if err := closeFn(); err != nil {
				errs = append(errs, err)
			}
		}
		e.closers = nil
		e.closeErr = errors.Join(errs...)
	})
	return e.closeErr
}

// InjectUserMessage delivers mid-turn steering: chat.Service flushes a queued
// steering message through here once the current turn ends, so the next
// StartTurn (or post-turn maintenance) sees it as part of the conversation.
// This is the engine's ONE message-history touchpoint — it's a turn-lifecycle
// concern, so it stays on the loop driver; the rest of history management
// (read/seed/count/truncate for fork/rollback/messages.list) is driven off
// [conversation.Service] directly by the runtime, never proxied through here.
func (e *Engine) InjectUserMessage(ctx context.Context, sessionID, text string) error {
	if e.steering == nil {
		return errors.New("engine: no steering port wired")
	}
	return e.steering.InjectUser(ctx, sessionID, text)
}
