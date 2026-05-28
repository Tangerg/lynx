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

// Config is the engine construction-time bundle. All fields are
// required — engine assumes its dependencies are wired before
// construction.
type Config struct {
	// ChatClient is the LLM client used by every action. Built from
	// a lynx model adapter (anthropic, openai, ...) at startup.
	ChatClient *chat.Client

	// Workdir is the filesystem root every Lyra-shipped tool is
	// scoped to. Empty string disables the scoping (LocalExecutor
	// permits any path) — fine for tests, not recommended for
	// production. Typical value: the user's project cwd.
	Workdir string

	// Online controls which network-reaching tools (webfetch /
	// websearch / httpreq) are registered. Each tool is independent;
	// missing credentials skip just that tool.
	Online OnlineConfig

	// MemoryStore optionally supplies a persistent chat-memory
	// backend (FileMessageStore, redis-backed, ...). When nil the
	// engine falls back to lynx's in-process [memory.InMemoryStore]
	// — fine for tests but loses history on restart.
	MemoryStore memory.Store

	// MemoryService optionally supplies the LYRA.md cascade reader
	// the agent injects into every system prompt. nil disables the
	// injection — the base prompt is used verbatim.
	MemoryService lyramem.Service

	// Compaction tunes the auto-compaction heuristic. Zero values
	// fall back to defaults (see [CompactionConfig]). Setting
	// MaxMessages negative disables auto-compaction entirely.
	Compaction CompactionConfig

	// MCPServers lists external MCP servers to dial at engine
	// construction. Their tools are merged into the built-in coding
	// tool set, prefixed with the server's Name so collisions across
	// servers stay separable. Empty disables MCP integration.
	MCPServers []MCPServer
}

// OnlineConfig is engine's view of the runtime-time online-tool
// credentials. Mirrors config.OnlineConfig but lives in this
// package so the engine has no dependency on the config layer
// (callers map between them).
type OnlineConfig struct {
	JinaAPIKey       string
	TavilyAPIKey     string
	HTTPAllowedHosts []string
}

// Engine is the runtime facade. It composes three concerns:
//
//   - chat execution: platform + agent drive [Engine.RunChat]
//   - maintenance:    compactor / extractor / planner power
//                     [Engine.MaybeCompact] / [Engine.MaybeExtract] /
//                     [Engine.GeneratePlan]
//   - context:        memStore / memSvc / workdir feed the system
//                     prompt and the chat-memory middleware
//
// Each sub-component is a focused struct in its own file; Engine
// just owns construction and the public surface. The chat.Engine
// interface in internal/service/chat narrows this to exactly the
// operations the chat service needs.
type Engine struct {
	// Chat execution.
	platform *runtime.Platform
	agent    *core.Agent

	// Context inputs (read at SystemPrompt + chat-memory time).
	tools    []chat.Tool
	memStore memory.Store
	memSvc   lyramem.Service
	workdir  string // captured from Config.Workdir for the AGENTS.md cascade

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
func New(cfg Config) (*Engine, error) {
	if cfg.ChatClient == nil {
		return nil, errors.New("engine: ChatClient is required")
	}

	tools, err := BuildToolSet(cfg.Workdir, cfg.Online)
	if err != nil {
		return nil, fmt.Errorf("engine: build tool set: %w", err)
	}

	// Dial MCP servers and merge their tools alongside the built-in
	// coding tools so the model can call them transparently. The
	// dial happens before resolver wiring so the resolver sees the
	// merged set in one place.
	mcpTools, mcpSessions, err := dialMCPServers(context.Background(), cfg.MCPServers)
	if err != nil {
		return nil, err
	}
	tools = append(tools, mcpTools...)
	resolver := buildCodingResolverFromTools(tools)

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
	})

	// Build the engine value first so the agent's Action closure can
	// capture *Engine (and therefore reach e.SystemPrompt) instead
	// of dragging a memory service through the constructor.
	e := &Engine{
		platform:    platform,
		tools:       tools,
		memStore:    memStore,
		memSvc:      cfg.MemoryService,
		workdir:     cfg.Workdir,
		mcpSessions: mcpSessions,
	}
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

// GeneratePlan asks the LLM for a step-by-step plan handling
// userMessage, threading in the same system prompt the regular
// agent would build (LYRA.md cascade + base persona). Returns an
// empty string when the LLM judges the request trivial (NO_PLAN).
//
// Lives on the engine — not the planner pointer directly — so
// chat.Service composes the prompt the same way the actual run
// will. Errors propagate as-is; the caller decides whether to
// fall back to direct execution.
func (e *Engine) GeneratePlan(ctx context.Context, userMessage string) (string, error) {
	return e.planner.Plan(ctx, e.SystemPrompt(ctx), userMessage)
}

// MaybeCompact runs one auto-compaction sweep against sessionID. The
// runtime calls this at every turn-end so growing histories get
// folded into a summary before the next turn starts. Returns
// (compacted, nil) — compacted is true only when the sweep
// actually replaced history, so callers can chain follow-on work
// (e.g. fact extraction) only on real events.
//
// No-op (returns false, nil) when:
//   - sessionID is empty (single-turn / no chat-memory path)
//   - the configured Compaction.MaxMessages is negative (disabled)
//   - the current history is shorter than the threshold
func (e *Engine) MaybeCompact(ctx context.Context, sessionID string) (bool, error) {
	return e.compactor.maybeCompact(ctx, sessionID)
}

// MaybeExtract mines the recent conversation for facts worth
// recording in <cwd>/LYRA.md. Best run right after MaybeCompact so
// the LLM sees a digest rather than a raw firehose. Best-effort —
// failures are logged but don't bubble.
//
// No-op when the engine has no MemoryService or the conversation
// is too short.
func (e *Engine) MaybeExtract(ctx context.Context, sessionID string) error {
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

// RunChatRequest carries the per-turn parameters for [Engine.RunChat].
// sessionID is non-empty to bind the turn to a chat-memory keyed
// conversation; observer is non-nil to receive streaming
// notifications.
type RunChatRequest struct {
	// SessionID anchors the turn to a chat-memory conversation. The
	// memory middleware reads this via [memory.ConversationIDKey] to
	// pull prior history before the model call and to save the new
	// round afterwards. Empty string runs the turn unattached (each
	// call starts fresh).
	SessionID string

	// Message is the user's input for this turn.
	Message string

	// Observer receives streaming tool-call + text-delta
	// notifications. May be nil — the turn still runs.
	Observer ToolObserver
}

// RunChat is the engine's lowest-level chat entry. The runtime
// schedules a process against the configured ChatAgent, blocks on
// completion, and returns the produced reply alongside the per-turn
// token roll-up.
//
// When req.Observer is non-nil the engine attaches a process-scope
// [core.ToolDecorator] that fires OnToolCallStart / OnToolCallEnd
// for every tool the model invokes during the turn, plus
// OnMessageDelta for each streamed text chunk.
//
// When req.SessionID is non-empty the engine binds the turn to a
// chat-memory keyed conversation — the memory middleware auto-loads
// prior turns and saves new messages keyed by SessionID.
func (e *Engine) RunChat(ctx context.Context, req RunChatRequest) (ChatOutput, error) {
	in := ChatInput{Message: req.Message}

	opts := core.ProcessOptions{}
	if req.SessionID != "" {
		opts.Session = &core.Session{ID: req.SessionID}
	}
	if req.Observer != nil {
		opts.Extensions = []core.Extension{
			&toolObserverDecorator{observer: req.Observer},
		}
	}

	proc, err := e.platform.RunAgent(ctx, e.agent,
		map[string]any{core.DefaultBindingName: in},
		opts,
	)
	if err != nil {
		return ChatOutput{}, fmt.Errorf("engine: run chat: %w", err)
	}
	out, ok := core.ResultOfType[ChatOutput](proc)
	if !ok {
		return ChatOutput{}, fmt.Errorf("engine: no ChatOutput produced; status=%s failure=%v", proc.Status(), proc.Failure())
	}
	return out, nil
}
