package engine

import (
	"context"
	"errors"
	"fmt"

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

// Engine is the runtime container. Holds the lynx Platform and the
// minimal Agent definition used for single-turn chat. Future
// milestones extend it (more agents, tool resolvers, blackboard
// providers, ...).
type Engine struct {
	platform  *runtime.Platform
	agent     *core.Agent
	tools     []chat.Tool
	memSvc    lyramem.Service
	compactor *compactor
	extractor *extractor
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
	resolver := buildCodingResolverFromTools(tools)

	memStore := cfg.MemoryStore
	if memStore == nil {
		memStore = memory.NewInMemoryStore()
	}
	callMW, streamMW, err := memory.NewMiddleware(memStore)
	if err != nil {
		return nil, fmt.Errorf("engine: build memory middleware: %w", err)
	}
	platform := agent.NewPlatform(&runtime.PlatformConfig{
		ChatClient: cfg.ChatClient,
		Extensions: []core.Extension{resolver},
		Guardrails: &core.Guardrails{
			CallMiddlewares:   []chat.CallMiddleware{callMW},
			StreamMiddlewares: []chat.StreamMiddleware{streamMW},
		},
	})

	a := buildChatAgent(cfg.MemoryService)
	if err := platform.Deploy(a); err != nil {
		return nil, fmt.Errorf("engine: deploy chat agent: %w", err)
	}

	var c *compactor
	if cfg.Compaction.MaxMessages >= 0 {
		c = newCompactor(memStore, cfg.ChatClient, cfg.Compaction)
	}

	var ex *extractor
	if cfg.MemoryService != nil {
		ex = newExtractor(memStore, cfg.MemoryService, cfg.ChatClient)
	}

	return &Engine{
		platform:  platform,
		agent:     a,
		tools:     tools,
		memSvc:    cfg.MemoryService,
		compactor: c,
		extractor: ex,
	}, nil
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
	if e.compactor == nil {
		return false, nil
	}
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
	if e.extractor == nil {
		return nil
	}
	return e.extractor.maybeExtract(ctx, sessionID)
}

// Tools returns the registered coding tool set — used by
// ToolService.List to surface tool metadata to clients without
// re-running the construction.
func (e *Engine) Tools() []chat.Tool { return e.tools }

// Platform exposes the underlying lynx platform for service
// implementations that need fine-grained control (most don't —
// prefer the high-level helpers below).
func (e *Engine) Platform() *runtime.Platform { return e.platform }

// ChatAgent returns the minimal "single-turn chat" agent. M2 adds
// tools to this agent's actions; M6 adds planner-aware variants.
func (e *Engine) ChatAgent() *core.Agent { return e.agent }

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
// completion, and returns the produced reply.
//
// When req.Observer is non-nil the engine attaches a process-scope
// [core.ToolDecorator] that fires OnToolCallStart / OnToolCallEnd
// for every tool the model invokes during the turn, plus
// OnMessageDelta for each streamed text chunk.
//
// When req.SessionID is non-empty the engine binds the turn to a
// chat-memory keyed conversation — the memory middleware auto-loads
// prior turns and saves new messages keyed by SessionID.
func (e *Engine) RunChat(ctx context.Context, req RunChatRequest) (string, error) {
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
		return "", fmt.Errorf("engine: run chat: %w", err)
	}
	out, ok := core.ResultOfType[ChatOutput](proc)
	if !ok {
		return "", fmt.Errorf("engine: no ChatOutput produced; status=%s failure=%v", proc.Status(), proc.Failure())
	}
	return out.Reply, nil
}
