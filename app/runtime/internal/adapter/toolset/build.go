package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/askuser"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/lsp"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/memorysearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/sessionsearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/shell"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sandbox"
)

// This file is the tool-assembly entry point. It is the SOLE place that
// constructs the capability adapters the tools wrap (code intelligence,
// background exec, MCP, A2A) and wires them into the resolver — so the engine
// CORE imports none of them; it receives the assembled [Built] from the
// composition root. Tool capability construction therefore stays outside Agent
// execution (doc/EXECUTION_CENTERED_ARCHITECTURE.md).

// BuildConfig is the tool-environment construction input (the working-directory
// scope + the capability tables). Driven by the runtime config.
type BuildConfig struct {
	Workdir       string
	UserHome      string
	DefaultModel  modelref.Selection
	SkillsUserDir string
	Online        OnlineConfig
	LSPServers    []codeintel.ServerSpec
	// MCPTools is the initial live MCP catalog. Its owner updates the resolver
	// after reconnects; toolset deliberately does not own MCP connections.
	MCPTools       []toolcontract.Tool
	A2AAgents      []A2AAgentConfig
	Plan           plan.Store      // backs set_plan + exit_plan_mode; nil → both are omitted
	PlanMode       plan.ModePolicy // session-scoped Plan mode; nil → enter/exit are omitted
	Interrupt      runs.InterruptFunc
	Schedules      schedule.Management     // backs schedule management tools; nil → omitted
	ToolResults    offload.Store           // backs read_tool_result (reads offloaded tool output); nil → omitted
	SkillUsage     skill.UsageRecorder     // records skill loads for the idle-lifecycle curator; nil → use recording off
	SkillProposals skill.ProposalSubmitter // backs root-only propose_skill; nil → omitted
	Goals          goal.State              // backs get_goal + report_goal_outcome and its active gate; nil → omitted

	// MemorySearch backs search_memory (keyword + semantic search over the
	// agent's curated project memory). nil omits the tool.
	MemorySearch memorysearch.Search

	// SessionSearch backs search_conversations (full-text search over past conversation
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
// runtime-scope resolver (also the diagnostic tool catalog) and the capability
// closers owned by bootstrap.Host.
type Built struct {
	Resolver *Resolver
	// Shells is the background-shell set the shell tools run over. Exposed so the
	// composition root can report a session's still-running jobs (e.g. a
	// post-compaction live-state reminder) without owning a second shell set.
	Shells  *exec.Shells
	Closers []func() error
}

// Build constructs every capability adapter, assembles the resolver, and
// returns the [Built] environment. Remote MCP connections are constructed by
// their dedicated adapter before this function and supplied as an initial tool
// snapshot; this package owns only the local and A2A capability lifecycle.
func Build(ctx context.Context, config BuildConfig) (_ Built, err error) {
	if config.Workdir == "" {
		return Built{}, errors.New("toolset: workdir is required")
	}
	if !filepath.IsAbs(config.Workdir) {
		return Built{}, errors.New("toolset: workdir must be absolute")
	}
	if config.UserHome == "" {
		return Built{}, errors.New("toolset: user home is required")
	}
	if !filepath.IsAbs(config.UserHome) {
		return Built{}, errors.New("toolset: user home must be absolute")
	}
	online, err := buildOnline(config.Online)
	if err != nil {
		return Built{}, err
	}

	// Code intelligence: one analyzer wrapping LSP clients; servers launch
	// lazily per (workspace root, language). Tools are cwd-independent (the
	// analyzer keys by root, read per call off the blackboard).
	codeIntel := codeintel.New(config.LSPServers)
	lspTools, err := lsp.Build(codeIntel, config.Workdir)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build lsp tools: %w", err)
	}

	tracker := newReadTracker()

	// OS command isolation for the shell tools. Build the confiner whenever the
	// host supports it — isolated sessions jail their shell even when the global
	// sandbox.shell opt-in is off. If the global opt-in IS on but the host has no
	// backend, that is a hard, fail-closed configuration error (refuse assembly).
	confiner, confErr := sandbox.NewConfiner(config.UserHome, config.SandboxReadOnlyPaths)
	if confErr != nil && config.SandboxShell {
		return Built{}, fmt.Errorf("toolset: enable shell sandbox: %w", confErr)
	}
	shells := exec.NewShells(confiner, config.SandboxShell)
	shellTools, err := shell.Build(shells, config.Workdir)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build shell tools: %w", err)
	}
	var a2aConns *a2a.Connections
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			err = errors.Join(err, shells.KillAll(), codeIntel.Close(), a2aConns.Close())
		}
	}()

	interrupt := config.Interrupt
	if interrupt == nil {
		interrupt = runs.InterruptUnavailable
	}

	// ask_user is a build-time tool shared by root and delegated roles. A child
	// question parks through the same nested suspension tree as child approval.
	askUserTool, err := askuser.New(interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build ask_user: %w", err)
	}

	// Plan mode and Plan state form one lifecycle. The family keeps
	// exit_plan_mode bound to the exact store maintained by set_plan.
	planFamily, err := plan.Build(config.PlanMode, config.Plan, interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build Plan family: %w", err)
	}
	scheduleTools, err := schedule.Build(config.Schedules)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build schedule tools: %w", err)
	}
	// read_tool_result reads back a tool output the runtime offloaded on
	// eviction. Working-directory independent (keys off the session id), so built
	// once and given to both roles. nil store → nil tool, simply omitted.
	toolResultTool, err := offload.New(config.ToolResults)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build read_tool_result: %w", err)
	}
	// search_memory reads back the agent's curated project memory (keyword +
	// semantic). Working-directory independent (searches the turn's project), so
	// built once for both roles. nil searcher → nil tool, simply omitted.
	memorySearchTool, err := memorysearch.New(config.MemorySearch)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build search_memory: %w", err)
	}
	// search_conversations recalls past conversation transcripts (full-text, all
	// sessions). Working-directory independent, so built once for both roles.
	// nil searcher → nil tool, simply omitted.
	sessionSearchTool, err := sessionsearch.New(config.SessionSearch)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build search_conversations: %w", err)
	}
	// Goal state is working-directory independent and keyed by session. get_goal
	// is always useful to the root Agent; report_goal_outcome is additionally
	// gated to an active Goal by the resolver.
	goalGetTool, err := goal.NewGet(config.Goals)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build get_goal: %w", err)
	}
	goalReportTool, err := goal.NewReport(config.Goals)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build report_goal_outcome: %w", err)
	}
	goalActive := goalActiveReader(config.Goals)
	proposeSkillTool, err := skill.NewProposal(config.SkillProposals, config.Workdir)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build propose_skill: %w", err)
	}

	connections, err := dialA2AConnections(ctx, config)
	a2aConns = connections.a2a
	if err != nil {
		return Built{}, err
	}

	resolver, err := newResolver(resolverDeps{
		SkillUsage:      config.SkillUsage,
		DefaultWorkdir:  config.Workdir,
		DefaultModel:    config.DefaultModel,
		SkillsUserDir:   config.SkillsUserDir,
		Online:          online,
		A2A:             connections.a2aTools,
		LSP:             lspTools,
		Shell:           shellTools,
		AskUser:         askUserTool,
		EnterPlan:       planFamily.Enter,
		ExitPlan:        planFamily.Exit,
		Plan:            planFamily.Set,
		ScheduleTools:   scheduleTools,
		ToolResult:      toolResultTool,
		MemorySearch:    memorySearchTool,
		SessionSearch:   sessionSearchTool,
		GoalGet:         goalGetTool,
		GoalReport:      goalReportTool,
		ProposeSkill:    proposeSkillTool,
		GoalActive:      goalActive,
		CodeIntel:       codeIntel,
		ReadTracker:     tracker,
		MCPToolDisabled: config.MCPToolDisabled,
	})
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build resolver: %w", err)
	}
	resolver.SetMCPTools(config.MCPTools) // seed the hot-swappable MCP set

	cleanupOnError = false
	return Built{
		Resolver: resolver,
		Shells:   shells,
		Closers: []func() error{
			codeIntel.Close,
			shells.KillAll,
			a2aConns.Close,
		},
	}, nil
}

// goalActiveReader adapts Goal state into the resolver's per-turn gate for
// report_goal_outcome. A paused or blocked Goal does not count: only a Run
// driven by the active autonomous loop can truthfully report its outcome.
// Returns nil when Goal mode is off so the tool is never offered.
func goalActiveReader(state goal.ActiveReader) func(context.Context, string) (bool, error) {
	if state == nil {
		return nil
	}
	return func(ctx context.Context, sessionID string) (bool, error) {
		return state.Active(ctx, sessionID)
	}
}
