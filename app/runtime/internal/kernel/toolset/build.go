package toolset

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset/askuser"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset/exitplan"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset/lsptools"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset/shell"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset/todotool"
)

// This file is the tool-assembly entry point. It is the SOLE place that
// constructs the capability adapters the tools wrap (code intelligence,
// background exec, MCP, A2A) and wires them into the resolver — so the engine
// CORE imports none of them; it receives the assembled [Built] from the
// composition root (runtime). This is the "tools assembled outside the core
// loop" shape the convergent microkernel design uses (doc/GREENFIELD_ARCHITECTURE.md §5.1).

// MCP wire projections, re-exported so the engine's MCP facade and the rpc
// layer name one type without importing infra/mcp.
type (
	MCPToolInfo     = mcp.ToolInfo
	MCPServerStatus = mcp.ServerStatus
)

// ErrUnknownMCPServer is returned by [MCPControl.Reconnect] for an
// unconfigured name (the wire layer maps it to invalid_params).
var ErrUnknownMCPServer = mcp.ErrUnknownServer

// MCPControl is the live-MCP-connections surface the engine exposes to
// workspace.mcp.* — implemented by the dialed connections.
type MCPControl interface {
	Statuses() []MCPServerStatus
	Tools(ctx context.Context, server string) ([]MCPToolInfo, error)
	Reconnect(ctx context.Context, name string) error
}

// BuildConfig is the tool-environment construction input (the working-directory
// scope + the capability tables). Driven by the runtime config.
type BuildConfig struct {
	Workdir         string
	SkillsGlobalDir string
	Online          OnlineConfig
	LSPServers      []codeintel.ServerSpec
	MCPServers      []mcp.ServerConfig
	A2AAgents       []a2a.ClientConfig
	Todos           todo.Service     // backs todo_write; nil → the tool is omitted
	Approval        approval.Service // backs exit_plan_mode (flips the stance on approval); nil → the tool is omitted
}

// Built is the assembled tool environment handed to the engine core: the
// platform-scope resolver, the canonical tool list (for tools.list — without
// the engine-built task/ask_user), the MCP control surface, and the capability
// closers the engine runs at shutdown.
type Built struct {
	Resolver *Resolver
	Tools    []chat.Tool
	MCP      MCPControl
	Closers  []func() error
}

// Build constructs every capability adapter, assembles the resolver, and
// returns the [Built] environment. A single unreachable MCP server is
// tolerated (recorded "failed"); a config mistake (duplicate name / invalid
// entry) fails. An A2A dial failure closes the already-opened MCP sessions so
// nothing leaks.
func Build(ctx context.Context, cfg BuildConfig) (Built, error) {
	online, err := BuildOnlineTools(cfg.Online)
	if err != nil {
		return Built{}, err
	}

	// Code intelligence: one service wrapping the LSP manager; servers launch
	// lazily per (workspace root, language). Tools are cwd-independent (the
	// service keys by root, read per call off the blackboard).
	codeIntel := codeintel.New(cfg.LSPServers)
	lspTools := lsptools.Build(codeIntel, cfg.Workdir)

	tracker := editguard.NewTracker()

	bg := exec.NewManager()
	shellTools := shell.Build(bg, cfg.Workdir)

	// ask_user is self-contained (SDK HITL + interrupts.Resolution), so it's a
	// plain build-time tool here, not engine-injected. Coding role only — the
	// resolver gates it (sub-agents don't supervise sub-process interrupts).
	askUserTool := askuser.New()

	// exit_plan_mode leaves the read-only plan stance: it presents the model's
	// plan for approval and, on approval, flips the approval stance to execute.
	// Self-contained (SDK HITL + the approval service), coding role only. nil
	// approval service → nil tool, simply omitted.
	exitPlanTool := exitplan.New(cfg.Approval)

	// todo_write maintains the per-session task list. nil cfg.Todos yields a nil
	// tool that's simply omitted (feature off). Working-directory independent
	// (keys off the session id), so built once and given to both roles.
	todoTool := todotool.New(cfg.Todos)

	mcpConns, mcpTools, err := mcp.Dial(ctx, cfg.MCPServers)
	if err != nil {
		return Built{}, err
	}

	a2aConns, a2aTools, err := a2a.Dial(ctx, cfg.A2AAgents)
	if err != nil {
		_ = mcpConns.Close()
		return Built{}, err
	}

	resolver := NewResolver(Deps{
		DefaultWorkdir:  cfg.Workdir,
		SkillsGlobalDir: cfg.SkillsGlobalDir,
		Online:          online,
		A2A:             a2aTools,
		LSP:             lspTools,
		Shell:           shellTools,
		AskUser:         askUserTool,
		ExitPlan:        exitPlanTool,
		Todo:            todoTool,
		CodeIntel:       codeIntel,
		ReadTracker:     tracker,
	})
	resolver.SetMCPTools(mcpTools)             // seed the hot-swappable MCP set
	mcpConns.SetToolSink(resolver.SetMCPTools) // reconnect hot-swaps the refreshed set in

	// Canonical tool list for tools.list — metadata (name/schema) is
	// working-directory independent, so the default-workdir build is faithful.
	// Only `task` is appended by the engine (it needs the platform).
	tools := append(BuildWorkdirTools(cfg.Workdir, codeIntel, tracker), online...)
	tools = append(tools, mcpTools...)
	tools = append(tools, a2aTools...)
	tools = append(tools, lspTools...)
	tools = append(tools, shellTools...)
	tools = append(tools, askUserTool)
	if skillTool := skill.Build(cfg.Workdir, cfg.SkillsGlobalDir); skillTool != nil {
		tools = append(tools, skillTool)
	}
	if todoTool != nil {
		tools = append(tools, todoTool)
	}

	return Built{
		Resolver: resolver,
		Tools:    tools,
		MCP:      mcpConns,
		Closers: []func() error{
			codeIntel.Close,
			func() error { bg.KillAll(); return nil },
			mcpConns.Close,
			a2aConns.Close,
		},
	}, nil
}
