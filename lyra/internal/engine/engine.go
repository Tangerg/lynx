package engine

import (
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
	lyramem "github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// Engine is the runtime facade. It composes three concerns:
//
//   - chat execution: platform + agent drive [Engine.StartChat]
//     (async; returns a [ChatProcess] handle backed by a real
//     [runtime.AgentProcess]) and [Engine.RunChat] (sync wrapper) —
//     see chatturn.go / chatprocess.go
//   - maintenance:    compactor / extractor / planner power
//     [Engine.MaybeCompact] / [Engine.MaybeExtract]
//   - context:        memStore / memSvc / workdir feed the system
//     prompt and the chat-memory middleware
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
	tools    []chat.Tool
	memStore memory.Store
	memSvc   lyramem.Service
	workdir  string  // captured from Config.Workdir for the AGENTS.md cascade
	pricing  Pricing // optional per-round cost hook; nil → cost stays zero

	// Maintenance sub-components — each may be nil when the
	// corresponding feature is disabled by config (e.g. extractor
	// when no MemoryService was supplied).
	compactor *compactor
	extractor *extractor
	planner   *planner

	// External lifecycle. mcpSessions are closed during [Engine.Close];
	// nil when no MCP servers are wired.
	mcpSessions []*sdkmcp.ClientSession
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

	// Dial MCP servers so the model can call them transparently. The dial
	// happens before resolver wiring so the resolver captures the set in
	// one place. ctx flows from the caller so a slow / unreachable MCP
	// server can be canceled during startup.
	mcpTools, mcpSessions, err := dialMCPServers(ctx, cfg.MCPServers)
	if err != nil {
		return nil, err
	}

	// One platform-scope resolver serves both roles. ToolRoleSubtask
	// resolves the cwd-bound coding tools + online + MCP; ToolRoleCoding
	// adds the `task` delegation tool, wired below once it exists (it
	// needs the platform). The resolver reads each turn's working
	// directory off the process blackboard at resolution time.
	resolver := &cwdToolResolver{
		defaultWorkdir: cfg.Workdir,
		online:         online,
		mcp:            mcpTools,
	}

	memStore := cfg.MemoryStore
	if memStore == nil {
		memStore = memory.NewInMemoryStore()
	}
	callMW, streamMW, err := memory.NewMiddleware(memStore)
	if err != nil {
		return nil, fmt.Errorf("engine: build memory middleware: %w", err)
	}
	platform := agent.NewPlatform(runtime.PlatformConfig{
		ChatClient: cfg.ChatClient,
		Extensions: []core.Extension{resolver},
		Guardrails: &core.Guardrails{
			CallMiddlewares:   []chat.CallMiddleware{callMW},
			StreamMiddlewares: []chat.StreamMiddleware{streamMW},
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
		platform:    platform,
		memStore:    memStore,
		memSvc:      cfg.MemoryService,
		workdir:     cfg.Workdir,
		pricing:     cfg.Pricing,
		mcpSessions: mcpSessions,
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
	e.tools = append(buildWorkdirTools(cfg.Workdir), online...)
	e.tools = append(e.tools, mcpTools...)
	e.tools = append(e.tools, taskTool)
	e.tools = append(e.tools, newAskUserTool())

	e.agent = e.buildChatAgent()
	if err := platform.Deploy(e.agent); err != nil {
		return nil, fmt.Errorf("engine: deploy chat agent: %w", err)
	}

	if cfg.Compaction.MaxMessages >= 0 {
		e.compactor = newCompactor(memStore, cfg.ChatClient, cfg.Compaction)
	}
	if cfg.MemoryService != nil {
		e.extractor = newExtractor(memStore, cfg.MemoryService, cfg.ChatClient)
	}
	e.planner = newPlanner(cfg.ChatClient)
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
	return e.compactor.maybeCompact(ctx, sessionID)
}

// MaybeExtract mines the recent conversation for facts worth
// recording in <cwd>/LYRA.md. Best run right after MaybeCompact so
// the LLM sees a digest rather than a raw firehose. The returned
// [ExtractionResult] reports whether anything was written and the
// facts themselves, so callers can surface a memory-updated event.
//
// No-op (zero ExtractionResult) when the engine has no MemoryService
// or the conversation is too short.
func (e *Engine) MaybeExtract(ctx context.Context, sessionID string) (ExtractionResult, error) {
	return e.extractor.maybeExtract(ctx, sessionID)
}

// Tools returns the registered coding tool set — used by
// ToolService.List to surface tool metadata to clients without
// re-running the construction.
func (e *Engine) Tools() []chat.Tool { return e.tools }

// Close releases per-engine external resources — currently only
// the MCP client sessions opened in [New]. Safe to call multiple
// times; the second call is a no-op.
//
// Errors from individual session closures are collected and
// returned together so the caller can log them; partial failure
// does not stop subsequent closes.
func (e *Engine) Close() error {
	if len(e.mcpSessions) == 0 {
		return nil
	}
	var errs []error
	for _, sess := range e.mcpSessions {
		if err := sess.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	e.mcpSessions = nil
	return errors.Join(errs...)
}

// InjectUserMessage appends a synthetic user message to the
// chat-memory store under sessionID. The message becomes part of
// the conversation history that the chat-memory middleware loads
// at the start of the next chat call.
//
// chat.Service.InjectSteering uses this to deliver mid-turn
// steering: the runtime queues the message in the active turn
// state and flushes it through here once the current turn ends
// in TurnEndCompleted so the next StartTurn (or post-turn
// maintenance, e.g. compaction) sees the steering as part of the
// conversation.
//
// Returns an error when sessionID is empty (no conversation to
// attach to) or text is empty (no message to inject).
func (e *Engine) InjectUserMessage(ctx context.Context, sessionID, text string) error {
	if sessionID == "" {
		return errors.New("engine: sessionID is required")
	}
	if text == "" {
		return errors.New("engine: text must not be empty")
	}
	return e.memStore.Write(ctx, sessionID, chat.NewUserMessage(text))
}

// ReadHistory returns the persisted chat-memory history for sessionID
// — the same messages the chat-memory middleware loads at the start of
// each turn. Empty (nil, nil) for an unknown / never-used session. The
// messages.list wire surface converts these to protocol.Message; fork
// copies a prefix of them. Reads through the engine (not the raw store)
// so callers depend on the engine's narrow surface, not memory.Store.
func (e *Engine) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	if sessionID == "" {
		return nil, errors.New("engine: sessionID is required")
	}
	return e.memStore.Read(ctx, sessionID)
}

// SeedHistory writes msgs into sessionID's chat-memory store. Used by
// sessions.fork to copy a slice of the parent's history into a freshly
// created child so the child's next turn continues from the fork point.
// No-op for an empty slice. The store appends, so seed a fresh session
// only (seeding one with existing history would concatenate).
func (e *Engine) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	if sessionID == "" {
		return errors.New("engine: sessionID is required")
	}
	if len(msgs) == 0 {
		return nil
	}
	return e.memStore.Write(ctx, sessionID, msgs...)
}
