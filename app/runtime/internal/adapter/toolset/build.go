package toolset

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/tools/httpreq"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/askuser"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/codebasesearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/exitplan"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/goaltool"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/lsptools"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/memorysearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/sessionsearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/shell"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skillpropose"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/todotool"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sandbox"
)

// This file is the tool-assembly entry point. It is the SOLE place that
// constructs the capability adapters the tools wrap (code intelligence,
// background exec, MCP, A2A) and wires them into the resolver — so the engine
// CORE imports none of them; it receives the assembled [Built] from the
// composition root. Tool capability construction therefore stays outside Agent
// execution (doc/EXECUTION_CENTERED_ARCHITECTURE.md).

// CodebaseIndex is the live @codebase capability the tool resolver consumes.
type CodebaseIndex interface {
	codebasesearch.SearchIndex
	Available(ctx context.Context) (bool, error)
}

// BuildConfig is the tool-environment construction input (the working-directory
// scope + the capability tables). Driven by the runtime config.
type BuildConfig struct {
	Workdir         string
	SkillsGlobalDir string
	Online          OnlineConfig
	LSPServers      []codeintel.ServerSpec
	MCPServers      []mcpserver.LiveConfig
	A2AAgents       []A2AAgentConfig
	Todos           todo.Store      // backs todo_write; nil → the tool is omitted
	Approval        approval.Policy // backs exit_plan_mode (flips the stance on approval); nil → the tool is omitted
	Interrupt       interrupts.Func
	Schedules       *schedules.Coordinator // backs the schedule tool; nil → omitted
	ToolResults     toolresult.Store       // backs read_tool_result (reads offloaded tool output); nil → omitted
	SkillAuthoring  skillpropose.Authoring // backs propose_skill (staged draft + human-gated promotion); nil/disabled → omitted
	SkillUsage      skill.UsageRecorder    // records skill loads for the idle-lifecycle curator; nil → use recording off
	Goals           goal.Store             // backs update_goal + gates it on an active goal (Goal mode); nil → omitted

	// CodebaseIndex backs codebase_search (semantic code search). nil — or an
	// index with no embedding model configured — omits the tool.
	CodebaseIndex CodebaseIndex

	// MemorySearch backs memory_search (keyword + semantic search over the
	// agent's curated project memory). nil omits the tool.
	MemorySearch memorysearch.Search

	// SessionSearch backs session_search (full-text search over past conversation
	// transcripts). nil omits the tool.
	SessionSearch sessionsearch.Search

	// MCPToolDisabled reports whether an identified MCP tool is hidden. The
	// runtime updates the underlying policy after every registry change.
	MCPToolDisabled func(mcpserver.ToolRef) bool

	// SandboxShell opts the shell tools into per-command OS isolation: each
	// command runs in an in-place jail rooted at its own cwd (workspace-write
	// only, network denied, $HOME hidden, env scrubbed). Off by default; on a
	// host with no isolation backend enabling it fails the build (fail-closed).
	SandboxShell bool
	// SandboxReadOnlyPaths re-opens declared toolchain roots below the hidden
	// home for reads (e.g. a language toolchain or dependency cache under $HOME).
	// Ignored unless SandboxShell is set.
	SandboxReadOnlyPaths []string
}

// Built is the assembled tool environment handed to the composition root: the
// runtime-scope resolver (also the diagnostic tool catalog), the live MCP ports,
// and the capability closers owned by bootstrap.Host.
type Built struct {
	Resolver              *Resolver
	MCPStatusReader       integrations.MCPStatusReader
	MCPToolCatalog        integrations.MCPToolCatalog
	MCPConnectionCommands integrations.MCPConnectionCommands
	MCPRegistryCommands   integrations.MCPRegistryCommands
	// Shells is the background-shell set the shell tools run over. Exposed so the
	// composition root can report a session's still-running jobs (e.g. a
	// post-compaction live-state reminder) without owning a second shell set.
	Shells  *exec.Shells
	Closers []func() error
}

// Build constructs every capability adapter, assembles the resolver, and
// returns the [Built] environment. A single unreachable MCP server is
// tolerated (recorded "failed"); a config mistake (duplicate name / invalid
// entry) fails. An A2A dial failure closes the already-opened MCP sessions so
// nothing leaks.
func Build(ctx context.Context, config BuildConfig) (_ Built, err error) {
	online, err := BuildOnlineTools(config.Online)
	if err != nil {
		return Built{}, err
	}

	// downloadAllow gates + guards the download tool: it shares httpreq's host
	// allowlist (a download is an arbitrary-URL GET that also writes to disk).
	downloadAllow, err := httpreq.NewAllowlist(config.Online.HTTPAllowedHosts)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: download allowlist: %w", err)
	}

	// Code intelligence: one analyzer wrapping LSP clients; servers launch
	// lazily per (workspace root, language). Tools are cwd-independent (the
	// analyzer keys by root, read per call off the blackboard).
	codeIntel := codeintel.New(config.LSPServers)
	lspTools, err := lsptools.Build(codeIntel, config.Workdir)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build lsp tools: %w", err)
	}

	tracker := editguard.NewTracker()

	// Opt-in per-command OS isolation for the shell tools. Built fail-closed: an
	// unsupported host refuses assembly rather than running the shell unconfined.
	var confiner *sandbox.Confiner
	if config.SandboxShell {
		confiner, err = sandbox.NewConfiner(config.SandboxReadOnlyPaths)
		if err != nil {
			return Built{}, fmt.Errorf("toolset: enable shell sandbox: %w", err)
		}
	}
	shells := exec.NewShells(confiner)
	shellTools, err := shell.Build(shells, config.Workdir)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build shell tools: %w", err)
	}
	var mcpConns *mcp.Connections
	var a2aConns *a2a.Connections
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			err = errors.Join(err, shells.KillAll(), codeIntel.Close(), mcpConns.Close(), a2aConns.Close())
		}
	}()

	interrupt := config.Interrupt
	if interrupt == nil {
		interrupt = interrupts.Unavailable
	}

	// ask_user is a build-time tool shared by root and subtask roles. A child
	// question parks through the same nested suspension tree as child approval.
	askUserTool, err := askuser.New(interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build ask_user: %w", err)
	}

	// exit_plan_mode leaves the read-only plan stance: it presents the model's
	// plan for approval and, on approval, flips the approval stance to execute.
	// Nil approval policy → nil tool, simply omitted.
	exitPlanTool, err := exitplan.New(config.Approval, interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build exit_plan_mode: %w", err)
	}

	// todo_write maintains the per-session task list. nil config.Todos yields a nil
	// tool that's simply omitted (feature off). Working-directory independent
	// (keys off the session id), so built once and given to both roles.
	todoTool, err := todotool.New(config.Todos)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build todo_write: %w", err)
	}
	scheduleTool, err := newScheduleTool(config.Schedules)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build schedule tool: %w", err)
	}
	// read_tool_result reads back a tool output the runtime offloaded on
	// eviction. Working-directory independent (keys off the session id), so built
	// once and given to both roles. nil store → nil tool, simply omitted.
	toolResultTool, err := toolresult.New(config.ToolResults)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build read_tool_result: %w", err)
	}
	// memory_search reads back the agent's curated project memory (keyword +
	// semantic). Working-directory independent (searches the turn's project), so
	// built once for both roles. nil searcher → nil tool, simply omitted.
	memorySearchTool, err := memorysearch.New(config.MemorySearch)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build memory_search: %w", err)
	}
	// session_search recalls past conversation transcripts (full-text, all
	// sessions). Working-directory independent, so built once for both roles.
	// nil searcher → nil tool, simply omitted.
	sessionSearchTool, err := sessionsearch.New(config.SessionSearch)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build session_search: %w", err)
	}
	// propose_skill lets the agent suggest a new reusable skill, gated behind a
	// human approval before it joins the global library. Root/coding role only.
	// nil / disabled authoring store → nil tool, simply omitted.
	skillProposeTool, err := skillpropose.New(config.SkillAuthoring, interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build propose_skill: %w", err)
	}
	// update_goal is the autonomous loop's completion signal (Goal mode). nil
	// store → nil tool + nil gate, so the tool never appears. Working-directory
	// independent (keys off the session id).
	goalTool, err := goaltool.New(config.Goals)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build update_goal: %w", err)
	}
	goalActive := goalActiveReader(config.Goals)

	mcpConns, mcpTools, err := mcp.Dial(ctx, infraMCPServerConfigs(config.MCPServers))
	if err != nil {
		return Built{}, err
	}

	a2aConns, a2aTools, err := a2a.Dial(ctx, infraA2AClientConfigs(config.A2AAgents))
	if err != nil {
		return Built{}, err
	}

	resolver, err := NewResolver(Deps{
		SkillUsage:      config.SkillUsage,
		DefaultWorkdir:  config.Workdir,
		SkillsGlobalDir: config.SkillsGlobalDir,
		Online:          online,
		A2A:             a2aTools,
		LSP:             lspTools,
		Shell:           shellTools,
		AskUser:         askUserTool,
		ExitPlan:        exitPlanTool,
		Todo:            todoTool,
		Schedule:        scheduleTool,
		ToolResult:      toolResultTool,
		MemorySearch:    memorySearchTool,
		SessionSearch:   sessionSearchTool,
		SkillPropose:    skillProposeTool,
		GoalUpdate:      goalTool,
		GoalActive:      goalActive,
		CodeIntel:       codeIntel,
		ReadTracker:     tracker,
		MCPToolDisabled: config.MCPToolDisabled,
		CodebaseIndex:   config.CodebaseIndex,
		DownloadAllow:   downloadAllow,
	})
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build resolver: %w", err)
	}
	resolver.SetMCPTools(mcpTools)             // seed the hot-swappable MCP set
	mcpConns.SetToolSink(resolver.SetMCPTools) // reconnect hot-swaps the refreshed set in

	// Canonical tool list for tools.list — metadata (name/schema) is
	// working-directory independent, so the default-workdir build is faithful.
	// Only `task` is appended by the engine (it needs the engine).
	tools := resolver.workdirTools(config.Workdir)
	tools = append(tools, online...)
	tools = append(tools, mcpTools...)
	tools = append(tools, a2aTools...)
	tools = append(tools, lspTools...)
	tools = append(tools, shellTools...)
	tools = append(tools, askUserTool)
	if skillTool := skill.Build(config.Workdir, config.SkillsGlobalDir, config.SkillUsage); skillTool != nil {
		tools = append(tools, skillTool)
	}
	if todoTool != nil {
		tools = append(tools, todoTool)
	}
	if scheduleTool != nil {
		tools = append(tools, scheduleTool)
	}
	if toolResultTool != nil {
		tools = append(tools, toolResultTool)
	}
	if memorySearchTool != nil {
		tools = append(tools, memorySearchTool)
	}
	if sessionSearchTool != nil {
		tools = append(tools, sessionSearchTool)
	}
	if skillProposeTool != nil {
		tools = append(tools, skillProposeTool)
	}
	// codebase_search is in the catalog whenever the index is wired — the tool's
	// metadata is meaningful regardless of the live embedding model, and the
	// per-turn resolver is the single live gate (it omits the tool until an
	// embedding model resolves). Gating the static catalog on Available() instead
	// would both miss a model configured after startup and resolve an embedding
	// client at construction.
	if config.CodebaseIndex != nil {
		codebaseSearch, err := codebasesearch.New(config.CodebaseIndex)
		if err != nil {
			return Built{}, fmt.Errorf("toolset: build codebase_search: %w", err)
		}
		tools = append(tools, codebaseSearch)
	}
	resolver.setCatalog(tools)

	mcpControl := &mcpControl{inner: mcpConns}

	cleanupOnError = false
	return Built{
		Resolver:              resolver,
		MCPStatusReader:       mcpControl,
		MCPToolCatalog:        mcpControl,
		MCPConnectionCommands: mcpControl,
		MCPRegistryCommands:   mcpControl,
		Shells:                shells,
		Closers: []func() error{
			codeIntel.Close,
			shells.KillAll,
			mcpConns.Close,
			a2aConns.Close,
		},
	}, nil
}

// goalActiveReader adapts the goal store into the resolver's per-turn gate for
// update_goal: it reports whether the session currently has an ACTIVE goal (a
// paused/blocked one does not count — the tool is only useful while the loop is
// driving). Returns nil when Goal mode is off so the tool is never offered.
func goalActiveReader(store goal.Store) func(context.Context, string) (bool, error) {
	if store == nil {
		return nil
	}
	return func(ctx context.Context, sessionID string) (bool, error) {
		g, ok, err := store.Get(ctx, sessionID)
		if err != nil {
			return false, err
		}
		return ok && g.Status == goal.StatusActive, nil
	}
}
