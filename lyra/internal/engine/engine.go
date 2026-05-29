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
	"github.com/Tangerg/lynx/mcp"
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
	MCPServers []mcp.ServerConfig

	// Pricing optionally computes per-round USD cost from the served
	// model + token usage. nil leaves cost at zero (the chat path gets
	// no dollar figure from providers). Supply a rate table to surface
	// CostUSD on ChatOutput / TurnEnd. See [Pricing].
	Pricing Pricing

	// ProcessStore, when non-nil, makes the platform auto-snapshot every
	// agent process per tick to a durable backend (audit trail + the
	// foundation for resuming a paused turn across restart). nil = no
	// persistence (no per-tick disk churn). The snapshot is process-level
	// (status / blackboard / history / budget), so for a single-action
	// chat turn it captures the turn boundary, not mid-LLM-loop state.
	ProcessStore core.ProcessStore
}

// OnlineConfig groups the credentials network-reaching tools need
// (webfetch / websearch / httpreq). Empty fields disable the
// corresponding tool — no tool is registered without explicit
// opt-in, so an offline-only install makes no surprise outbound
// calls. This is the canonical definition; the config layer stores
// it directly (no bridge mapping), keeping engine free of any
// dependency on config.
type OnlineConfig struct {
	// JinaAPIKey enables the webfetch tool backed by Jina Reader.
	JinaAPIKey string

	// TavilyAPIKey enables the websearch tool backed by Tavily.
	TavilyAPIKey string

	// HTTPAllowedHosts enables the httpreq tool. Pass an explicit
	// allowlist (e.g. ["api.github.com", "*.openai.com"]) — empty
	// keeps the tool disabled so the LLM can't reach arbitrary
	// internal endpoints.
	HTTPAllowedHosts []string
}

// Engine is the runtime facade. It composes three concerns:
//
//   - chat execution: platform + agent drive [Engine.StartChat]
//     (async; returns a [ChatProcess] handle backed by a real
//     [runtime.AgentProcess]) and [Engine.RunChat] (sync wrapper)
//   - maintenance:    compactor / extractor / planner power
//     [Engine.MaybeCompact] / [Engine.MaybeExtract] /
//     [Engine.GeneratePlan]
//   - context:        memStore / memSvc / workdir feed the system
//     prompt and the chat-memory middleware
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

	tools, err := BuildToolSet(cfg.Workdir, cfg.Online)
	if err != nil {
		return nil, fmt.Errorf("engine: build tool set: %w", err)
	}

	// Dial MCP servers and merge their tools alongside the built-in
	// coding tools so the model can call them transparently. The
	// dial happens before resolver wiring so the resolver sees the
	// merged set in one place. ctx flows from the caller so a slow /
	// unreachable MCP server can be canceled during startup.
	mcpTools, mcpSessions, err := dialMCPServers(ctx, cfg.MCPServers)
	if err != nil {
		return nil, err
	}
	leafTools := append(tools, mcpTools...)

	// Two tool roles share the leaf coding set. ToolRoleSubtask gets
	// exactly those (registered now); ToolRoleCoding gets them plus the
	// `task` delegation tool, registered below once that tool exists
	// (it needs the platform). Register is last-write-wins.
	resolver := core.NewStaticToolGroupResolver("lyra-tools")
	resolver.Register(ToolRoleSubtask, newToolGroup(ToolRoleSubtask, leafTools))

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
	// ToolRoleSubtask → leaf tools, no `task` → no recursion). Fold it
	// into the main coding role. AsChatToolFromAgent needs no separate
	// deploy — child processes land on the platform when spawned.
	taskTool, err := runtime.AsChatToolFromAgent[TaskInput, string](platform, e.buildSubtaskAgent())
	if err != nil {
		return nil, fmt.Errorf("engine: build task tool: %w", err)
	}
	// Full-slice expression caps leafTools so this append allocates a
	// fresh array — the ToolRoleSubtask group keeps its leaf-only view.
	codingTools := append(leafTools[:len(leafTools):len(leafTools)], taskTool)
	resolver.Register(ToolRoleCoding, newToolGroup(ToolRoleCoding, codingTools))
	e.tools = codingTools

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

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. See
	// [ChatInput.MaxBudget] for the stop semantics.
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost (0 = no cap). See
	// [ChatInput.MaxCostUSD] — requires a [Config.Pricing] hook.
	MaxCostUSD float64

	// PlanMode runs the turn behind plan approval — see
	// [ChatInput.PlanMode]. The process parks on AwaitInput after
	// drafting a plan; drive it back with [ChatProcess.Resume].
	PlanMode bool

	// Observer receives streaming tool-call + text-delta
	// notifications. May be nil — the turn still runs.
	Observer ToolObserver

	// EventListener, when non-nil, is registered as a process-scope
	// extension. Values that also implement [event.Listener] (i.e.
	// have OnEvent) receive every agent runtime event for this turn
	// — process lifecycle (Created / Completed / Failed / Killed /
	// Stuck / Terminated), action execution, ready-to-plan, etc.
	// The canonical wrapper is [event.NamedListener]; chat.Service
	// uses one to map process terminal events onto TurnEnd reasons
	// without re-deriving status from the run loop's error.
	//
	// Names must be unique across the process extension slice — the
	// runtime panics on collisions at registration time.
	EventListener core.Extension
}

// ChatProcess is the handle [Engine.StartChat] returns. It exposes
// the underlying [runtime.AgentProcess] lifecycle (status, failure,
// cancellation) plus a typed result extractor — chat.Service drives
// the turn off Done() and queries Status() to decide TurnEnd reason.
//
// The interface lives in this package (not chat.Engine) so test
// stubs can substitute a fake without standing up a full platform.
type ChatProcess interface {
	// ID is the underlying agent process id — surfaces to clients as
	// the turn handle so cancellation / resume requests route through
	// the runtime by process id.
	ID() string

	// Status reports the current [core.AgentProcessStatus] —
	// Running while the action loop ticks, Completed / Failed /
	// Killed / Terminated when the run ends.
	Status() core.AgentProcessStatus

	// Failure returns the terminal error the process recorded on
	// itself, or nil when the run is still in flight or succeeded.
	Failure() error

	// Done delivers the final error (or nil on success) once the
	// run loop exits. Buffered cap-1 so callers can receive after
	// the goroutine has already finished.
	Done() <-chan error

	// Output extracts the typed [ChatOutput] from the process
	// blackboard. Returns an error when the run produced no output
	// (status reflects the terminal cause).
	Output() (ChatOutput, error)

	// Cancel marks the process [core.StatusKilled] via the platform.
	// The ongoing tick observes the status flip at its next checkpoint
	// and the run loop exits, delivering its error on Done().
	Cancel(reason string) error

	// Resume answers a plan-mode approval the process is parked on
	// (StatusWaiting): it delivers the decision and continues the
	// process, returning a fresh Done channel for the resumed run
	// (which executes the plan when approved, or produces a
	// PlanRejected output when not). Only valid while Status is
	// [core.StatusWaiting].
	Resume(ctx context.Context, approved bool) (<-chan error, error)
}

// chatProcess is the canonical [ChatProcess] backed by a real
// [runtime.AgentProcess]. Platform reference is held so Cancel can
// invoke [runtime.Platform.KillProcess] without callers reaching
// into engine internals.
type chatProcess struct {
	proc     *runtime.AgentProcess
	done     <-chan error
	platform *runtime.Platform
}

func (cp *chatProcess) ID() string                      { return cp.proc.ID() }
func (cp *chatProcess) Status() core.AgentProcessStatus { return cp.proc.Status() }
func (cp *chatProcess) Failure() error                  { return cp.proc.Failure() }
func (cp *chatProcess) Done() <-chan error              { return cp.done }
func (cp *chatProcess) Cancel(reason string) error {
	_ = reason
	return cp.platform.KillProcess(cp.proc.ID())
}

func (cp *chatProcess) Resume(ctx context.Context, approved bool) (<-chan error, error) {
	if _, err := cp.platform.ResumeProcess(cp.proc.ID(), approved); err != nil {
		return nil, err
	}
	return cp.platform.ContinueProcessAsync(ctx, cp.proc.ID()), nil
}

func (cp *chatProcess) Output() (ChatOutput, error) {
	out, ok := core.ResultOfType[ChatOutput](cp.proc)
	if !ok {
		return ChatOutput{}, fmt.Errorf("engine: no ChatOutput produced; status=%s failure=%v", cp.proc.Status(), cp.proc.Failure())
	}
	return out, nil
}

// StartChat dispatches a chat turn as an async agent process and
// returns the [ChatProcess] handle the caller drives. The lifecycle
// — cancel, status, awaiting completion, output extraction — runs
// against the agent runtime's [runtime.AgentProcess] rather than a
// bare goroutine, so future HITL integration (plan approval, tool
// approval) can drop in on the same Process via
// [runtime.Platform.ResumeProcess].
//
// Observer / SessionID wiring matches [Engine.RunChat]: Observer
// attaches a process-scope [core.ToolDecorator]; SessionID binds the
// turn to the chat-memory middleware's keyed conversation.
func (e *Engine) StartChat(ctx context.Context, req RunChatRequest) ChatProcess {
	in := ChatInput{Message: req.Message, MaxBudget: req.MaxBudget, MaxCostUSD: req.MaxCostUSD, PlanMode: req.PlanMode}

	opts := core.ProcessOptions{}
	if req.SessionID != "" {
		opts.Session = &core.Session{ID: req.SessionID}
	}
	if req.Observer != nil {
		opts.Extensions = append(opts.Extensions, &toolObserverDecorator{observer: req.Observer})
	}
	if req.EventListener != nil {
		opts.Extensions = append(opts.Extensions, req.EventListener)
	}

	proc, done := e.platform.StartAgent(ctx, e.agent,
		map[string]any{core.DefaultBindingName: in},
		opts,
	)
	return &chatProcess{proc: proc, done: done, platform: e.platform}
}

// RunChat is the synchronous wrapper kept for callers that don't
// need the [ChatProcess] handle (engine tests, CLI smoke runs).
// Newer call sites should use [Engine.StartChat] directly.
func (e *Engine) RunChat(ctx context.Context, req RunChatRequest) (ChatOutput, error) {
	cp := e.StartChat(ctx, req)
	if err := <-cp.Done(); err != nil {
		return ChatOutput{}, fmt.Errorf("engine: run chat: %w", err)
	}
	return cp.Output()
}
