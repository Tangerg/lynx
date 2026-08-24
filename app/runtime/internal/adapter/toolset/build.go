package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sandbox"
)

// This file is the tool-assembly entry point. It is the SOLE place that
// constructs the capability adapters the tools wrap (code intelligence,
// background exec, MCP, A2A) and wires them into the resolver — so the engine
// CORE imports none of them; it receives the assembled [Built]. Tool capability
// construction therefore stays outside Agent
// execution (doc/ARCHITECTURE.md).

// BuildConfig is the tool-environment construction input (the working-directory
// scope + the capability tables). Driven by the runtime config.
type BuildConfig struct {
	// Lifetime is the process-owned root for local capability resources that
	// outlive the startup call and individual Tool invocations.
	Lifetime      context.Context
	DefaultCWD    string
	UserHome      string
	SkillsUserDir string
	Online        OnlineConfig
	LSPServers    []codeintel.ServerSpec
	// MCPTools is the initial live MCP catalog. Its owner updates the resolver
	// after reconnects; toolset deliberately does not own MCP connections.
	MCPTools       []toolcontract.Tool
	A2AAgents      []A2AAgentConfig
	Plan           builtin.PlanUseCases   // backs set_plan + exit_plan_mode; nil → both are omitted
	PlanMode       builtin.PlanModePolicy // session-scoped Plan mode; nil → enter/exit are omitted
	Interrupt      runs.InterruptFunc
	Schedules      builtin.ScheduleManagement     // backs schedule management tools; nil → omitted
	ToolResults    builtin.ToolResultStore        // backs read_tool_result (reads offloaded tool output); nil → omitted
	SkillUsage     builtin.SkillUsageRecorder     // records skill loads for the idle-lifecycle curator; nil → use recording off
	SkillProposals builtin.SkillProposalSubmitter // backs root-only propose_skill; nil → omitted
	GoalReader     GoalReader                     // backs get_goal; nil → omitted
	GoalReporter   builtin.GoalOutcomeReporter    // backs report_goal_outcome; nil → omitted

	// AgentMemorySearch backs search_memory (keyword + semantic search over the
	// agent's curated memory visible from the current project context). nil omits
	// the tool.
	AgentMemorySearch builtin.AgentMemorySearch

	// ConversationSearch backs search_conversations (full-text search over past conversation
	// transcripts). nil omits the tool.
	ConversationSearch builtin.ConversationSearch

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

// GoalReader supplies the Goal read model used by get_goal. Outcome-tool
// availability is derived from immutable Run admission provenance instead of
// mutable Goal state.
type GoalReader interface {
	builtin.GoalReader
}

// Built is the assembled tool environment: the runtime-scope resolver (also the
// diagnostic tool catalog) and its capability closers.
type Built struct {
	Resolver *Resolver
	// Shells is the background-shell set the shell tools run over. Exposed so
	// live-state reporting can inspect a session's still-running jobs (e.g. a
	// post-compaction live-state reminder) without owning a second shell set.
	Shells  *exec.Shells
	Closers []func() error
}

// Build constructs every capability adapter, assembles the resolver, and
// returns the [Built] environment. Remote MCP connections are constructed by
// their dedicated adapter before this function and supplied as an initial tool
// snapshot; this package owns only the local and A2A capability lifecycle.
func Build(ctx context.Context, config BuildConfig) (_ Built, err error) {
	if ctx == nil {
		return Built{}, errors.New("toolset: startup context is required")
	}
	if config.Lifetime == nil {
		return Built{}, errors.New("toolset: lifetime is required")
	}
	if config.DefaultCWD == "" {
		return Built{}, errors.New("toolset: default CWD is required")
	}
	if !filepath.IsAbs(config.DefaultCWD) {
		return Built{}, errors.New("toolset: default CWD must be absolute")
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
	codeIntel, err := codeintel.New(config.Lifetime, config.LSPServers)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build code intelligence: %w", err)
	}
	lspTools, err := builtin.BuildLSP(codeIntel, config.DefaultCWD)
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
	shellTools, err := builtin.BuildShell(shells, config.DefaultCWD)
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

	// ask_user is a build-time Tool shared by root and delegated roles. A child
	// question waits at the same durable tree boundary as child approval.
	askUserTool, err := builtin.NewAskUser(interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build ask_user: %w", err)
	}

	// Plan mode and Plan state form one lifecycle. The family keeps
	// exit_plan_mode bound to the exact store maintained by set_plan.
	planFamily, err := builtin.BuildPlan(config.PlanMode, config.Plan, interrupt)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build Plan family: %w", err)
	}
	scheduleTools, err := builtin.BuildSchedules(config.Schedules)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build schedule tools: %w", err)
	}
	// read_tool_result reads back a tool output the runtime offloaded on
	// eviction. Working-directory independent (keys off the session id), so built
	// once and given to both roles. nil store → nil tool, simply omitted.
	toolResultTool, err := builtin.NewToolResultReader(config.ToolResults)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build read_tool_result: %w", err)
	}
	// search_memory reads back exact-project and user-scoped curated memory
	// (keyword + semantic). Working-directory independent (searches the Run's
	// project), so built once for both roles. nil searcher → nil tool, simply
	// omitted.
	agentMemorySearchTool, err := builtin.NewAgentMemorySearch(config.AgentMemorySearch)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build search_memory: %w", err)
	}
	// search_conversations recalls past conversation transcripts (full-text, all
	// sessions). Working-directory independent, so built once for both roles.
	// nil searcher → nil tool, simply omitted.
	conversationSearchTool, err := builtin.NewConversationSearch(config.ConversationSearch)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build search_conversations: %w", err)
	}
	// Goal state is working-directory independent and keyed by session. get_goal
	// is always useful to the root Agent; report_goal_outcome is exposed only to
	// Runs stamped with a Goal incarnation at admission. That immutable provenance
	// keeps a parked deployment restorable even while the Goal is paused for HITL.
	goalGetTool, err := builtin.NewGet(config.GoalReader)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build get_goal: %w", err)
	}
	goalReportTool, err := builtin.NewReport(config.GoalReporter)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build report_goal_outcome: %w", err)
	}
	proposeSkillTool, err := builtin.NewProposal(config.SkillProposals, config.DefaultCWD)
	if err != nil {
		return Built{}, fmt.Errorf("toolset: build propose_skill: %w", err)
	}

	connections, err := dialA2AConnections(ctx, config)
	a2aConns = connections.a2a
	if err != nil {
		return Built{}, err
	}

	resolver, err := newResolver(resolverDeps{
		SkillUsage:         config.SkillUsage,
		DefaultCWD:         config.DefaultCWD,
		SkillsUserDir:      config.SkillsUserDir,
		Online:             online,
		A2A:                connections.a2aTools,
		LSP:                lspTools,
		Shell:              shellTools,
		AskUser:            askUserTool,
		EnterPlan:          planFamily.Enter,
		ExitPlan:           planFamily.Exit,
		Plan:               planFamily.Set,
		ScheduleTools:      scheduleTools,
		ToolResult:         toolResultTool,
		AgentMemorySearch:  agentMemorySearchTool,
		ConversationSearch: conversationSearchTool,
		GoalGet:            goalGetTool,
		GoalReport:         goalReportTool,
		ProposeSkill:       proposeSkillTool,
		CodeIntel:          codeIntel,
		ReadTracker:        tracker,
		MCPToolDisabled:    config.MCPToolDisabled,
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
