package toolset

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/askuser"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/codebasesearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/exitplan"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/lsptools"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/shell"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/todotool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

// This file is the tool-assembly entry point. It is the SOLE place that
// constructs the capability adapters the tools wrap (code intelligence,
// background exec, MCP, A2A) and wires them into the resolver — so the engine
// CORE imports none of them; it receives the assembled [Built] from the
// composition root (runtime). This is the "tools assembled outside the core
// loop" shape the convergent microkernel design uses (doc/GREENFIELD_ARCHITECTURE.md §5.1).

// CodebaseIndex is the live @codebase capability the tool resolver consumes.
type CodebaseIndex interface {
	codebasesearch.SearchIndex
	Available(ctx context.Context) bool
}

// BuildConfig is the tool-environment construction input (the working-directory
// scope + the capability tables). Driven by the runtime config.
type BuildConfig struct {
	Workdir         string
	SkillsGlobalDir string
	Online          OnlineConfig
	LSPServers      []codeintel.ServerSpec
	MCPServers      []toolport.MCPServerConfig
	A2AAgents       []A2AAgentConfig
	Todos           todo.Store      // backs todo_write; nil → the tool is omitted
	Approval        approval.Policy // backs exit_plan_mode (flips the stance on approval); nil → the tool is omitted
	Interruption    interrupts.Interruption

	// CodebaseIndex backs codebase_search (semantic code search). nil — or an
	// index with no embedding model configured — omits the tool.
	CodebaseIndex CodebaseIndex

	// MCPDisabled returns the model-facing MCP tool names the configured servers
	// hide from the model (per-server blacklist; nil → no filtering). The runtime
	// recomputes it on every registry change; the resolver reads it per
	// resolution so a disable takes effect mid-session.
	MCPDisabled func() map[string]struct{}
}

// Built is the assembled tool environment handed to the engine core: the
// platform-scope resolver, the canonical tool list (for tools.list — without
// the engine-built task/ask_user), the live MCP ports, and the capability
// closers the engine runs at shutdown.
type Built struct {
	Resolver              *Resolver
	Tools                 []chat.Tool
	MCPStatusReader       toolport.MCPStatusReader
	MCPToolCatalog        toolport.MCPToolCatalog
	MCPConnectionCommands toolport.MCPConnectionCommands
	MCPRegistryCommands   toolport.MCPRegistryCommands
	Closers               []func() error
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

	// Code intelligence: one analyzer wrapping LSP clients; servers launch
	// lazily per (workspace root, language). Tools are cwd-independent (the
	// analyzer keys by root, read per call off the blackboard).
	codeIntel := codeintel.New(cfg.LSPServers)
	lspTools := lsptools.Build(codeIntel, cfg.Workdir)

	tracker := editguard.NewTracker()

	shells := exec.NewShells()
	shellTools := shell.Build(shells, cfg.Workdir)

	interrupt := cfg.Interruption
	if interrupt == nil {
		interrupt = interrupts.NoInterruption
	}

	// ask_user is build-time tool here, not engine-injected. Coding role only.
	askUserTool := askuser.New(interrupt)

	// exit_plan_mode leaves the read-only plan stance: it presents the model's
	// plan for approval and, on approval, flips the approval stance to execute.
	// Nil approval policy → nil tool, simply omitted.
	exitPlanTool := exitplan.New(cfg.Approval, interrupt)

	// todo_write maintains the per-session task list. nil cfg.Todos yields a nil
	// tool that's simply omitted (feature off). Working-directory independent
	// (keys off the session id), so built once and given to both roles.
	todoTool := todotool.New(cfg.Todos)

	mcpConns, mcpTools, err := mcp.Dial(ctx, infraMCPServerConfigs(cfg.MCPServers))
	if err != nil {
		return Built{}, err
	}

	a2aConns, a2aTools, err := a2a.Dial(ctx, infraA2AClientConfigs(cfg.A2AAgents))
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
		MCPDisabled:     cfg.MCPDisabled,
		CodebaseIndex:   cfg.CodebaseIndex,
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
	// codebase_search is in the catalog whenever the index is wired — the tool's
	// metadata is meaningful regardless of the live embedding model, and the
	// per-turn resolver is the single live gate (it omits the tool until an
	// embedding model resolves). Gating the static catalog on Available() instead
	// would both miss a model configured after startup and resolve an embedding
	// client at construction.
	if cfg.CodebaseIndex != nil {
		codebaseSearch, err := codebasesearch.New(cfg.CodebaseIndex)
		if err != nil {
			return Built{}, fmt.Errorf("toolset: build codebase_search: %w", err)
		}
		tools = append(tools, codebaseSearch)
	}

	mcpControl := &mcpControl{inner: mcpConns}

	return Built{
		Resolver:              resolver,
		Tools:                 tools,
		MCPStatusReader:       mcpControl,
		MCPToolCatalog:        mcpControl,
		MCPConnectionCommands: mcpControl,
		MCPRegistryCommands:   mcpControl,
		Closers: []func() error{
			codeIntel.Close,
			func() error { shells.KillAll(); return nil },
			mcpConns.Close,
			a2aConns.Close,
		},
	}, nil
}
