package toolset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/codebasesearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/editguardstate"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/toolsearch"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/tools/httpreq"
)

// The per-turn application-context seam (cwd, session, isolation, goal lease)
// lives in package executionctx — the resolver, per-tool packages, and prompt
// composition all read it inward without coupling to each other.

// Resolver is the engine-scope [core.ToolGroupResolver] for the
// coding + subtask roles. The working-directory-independent tools (online
// providers, MCP servers, the `task` delegation tool) are built once at
// engine construction and captured here; filesystem and skill tools are
// rebuilt per resolution, while shell and LSP tools read the resolving turn's
// application context per call. Both paths fall back to defaultWorkdir. That is
// what lets a single engine serve many sessions —
// each running its tools in its own project directory — without a
// per-session engine.
type Resolver struct {
	lateMu sync.RWMutex

	defaultWorkdir  string
	defaultModel    modelref.Selection
	skillsGlobalDir string                                      // user-scope skills dir; merged under each turn's project skills
	skillUsage      skill.UsageRecorder                         // records skill loads for the idle-lifecycle curator; nil → off
	online          []toolcontract.Tool                         // working-directory-independent network tools
	a2a             []toolcontract.Tool                         // working-directory-independent remote A2A agents
	lsp             []toolcontract.Tool                         // code-intelligence tools; cwd read per-call (analyzer keys servers by root)
	codeIntel       *codeintel.Analyzer                         // backs the write/edit diagnostics wrap (rebuilt per resolution with the turn's cwd)
	readTracker     *editguardstate.Tracker                     // backs the read-before-edit + stale guards on read/edit/write
	pathLocker      *pathLocker                                 // serializes same-path fs calls across every concurrent turn resolution
	shell           []toolcontract.Tool                         // shell tools (shell / shell_output / shell_kill) over the exec.Shells; cwd read per-call
	task            toolcontract.Tool                           // bounded recursive delegation tool; nil until set
	createGoal      toolcontract.Tool                           // root-only Goal entry tool; nil until the Goal Driver exists
	staticTools     []staticToolSpec                            // built-once tools with one role/placement policy for turn manifests
	goalActive      func(context.Context, string) (bool, error) // reports whether the session has an active Goal; nil → outcome reporting never offered

	// codebaseIndex backs codebase_search (both roles). Held as the index (not
	// a pre-built tool) so Tools() can gate inclusion on Available() per turn —
	// the embedding model can be configured after construction. nil → no tool.
	codebaseIndex CodebaseIndex

	// downloadAllow gates + guards the download tool (the same host allowlist as
	// httpreq). Empty → the tool is omitted from every resolution.
	downloadAllow httpreq.Allowlist

	// mcp is the working-directory-independent MCP tool set, held behind an
	// atomic pointer so a reconnect (B3b-2) can hot-swap the live set without
	// locking the per-turn resolution path: Tools() does one atomic load, the
	// reconnect does one atomic store. The model therefore always sees the
	// currently-connected servers' tools, even mid-session.
	mcp atomic.Pointer[[]toolcontract.Tool]

	// mcpToolDisabled reads the current domain policy per resolution so registry
	// changes and live-tool reconnects remain independent hot swaps.
	mcpToolDisabled func(mcpserver.ToolRef) bool
}

type toolAudience uint8

const (
	toolAudienceBoth toolAudience = iota
	toolAudienceCoding
)

func (a toolAudience) includes(role string) bool {
	return a == toolAudienceBoth || role == domaintool.GroupCoding
}

type toolPlacement uint8

const (
	toolAfterSkill toolPlacement = iota
	toolAfterCodebase
	toolCodingTail
)

// staticToolSpec is the policy table for built-once per-turn tools. A turn
// consumes entries in its placement and audience, evaluating the one dynamic
// active-goal condition without turning resolution into a generic registry.
type staticToolSpec struct {
	tool               toolcontract.Tool
	audience           toolAudience
	placement          toolPlacement
	deferred           bool
	requiresActiveGoal bool
}

// Deps bundles the working-directory-independent inputs the resolver captures
// at construction. Filesystem and skill tools are rebuilt per resolution;
// shell and LSP tools are built once but read the turn's cwd per call. Online,
// A2A, and code-intelligence capabilities are also built once and held.
type Deps struct {
	DefaultWorkdir  string
	DefaultModel    modelref.Selection
	SkillsGlobalDir string
	SkillUsage      skill.UsageRecorder
	Online          []toolcontract.Tool                         // network tools (webfetch/websearch/httpreq)
	A2A             []toolcontract.Tool                         // remote A2A delegation tools
	LSP             []toolcontract.Tool                         // code-intelligence tools
	Shell           []toolcontract.Tool                         // shell tools (shell / shell_output / shell_kill); nil means omitted
	AskUser         toolcontract.Tool                           // ask_user HITL tool (both roles)
	EnterPlan       toolcontract.Tool                           // enter_plan_mode (coding role only); nil → omitted
	ExitPlan        toolcontract.Tool                           // exit_plan_mode (coding role only); nil → omitted
	Plan            toolcontract.Tool                           // set_plan execution-plan tool (coding role only); nil → omitted
	Schedule        toolcontract.Tool                           // schedule management op-tool (coding role only); nil → omitted
	ToolResult      toolcontract.Tool                           // read_tool_result offloaded-output reader (both roles); nil → omitted
	MemorySearch    toolcontract.Tool                           // memory_search agent-memory reader (both roles); nil → omitted
	SessionSearch   toolcontract.Tool                           // session_search past-transcript reader (both roles); nil → omitted
	SkillPropose    toolcontract.Tool                           // propose_skill authoring tool (coding role only); nil → omitted
	GoalGet         toolcontract.Tool                           // get_goal state reader (coding role only); nil → omitted
	GoalReport      toolcontract.Tool                           // report_goal_outcome loop signal (coding role only); nil → omitted
	GoalActive      func(context.Context, string) (bool, error) // reports an active Goal for the session; nil → outcome reporting never offered
	CodeIntel       *codeintel.Analyzer                         // backs the post-edit diagnostics wrap
	ReadTracker     *editguardstate.Tracker                     // backs the read/edit/write guards
	CodebaseIndex   CodebaseIndex                               // backs codebase_search (both roles); nil → omitted
	DownloadAllow   httpreq.Allowlist                           // host allowlist gating/guarding download; empty → omitted

	// MCPToolDisabled reports whether an identified MCP tool is hidden.
	MCPToolDisabled func(mcpserver.ToolRef) bool
}

type mcpToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

// NewResolver builds the engine-scoped tool resolver from its
// working-directory-independent inputs. The `task` delegation and create_goal
// entry tools are injected through explicit seams after their cyclic runtime
// owners exist; the MCP tool set is seeded + hot-swapped via [Resolver.SetMCPTools].
func NewResolver(d Deps) (*Resolver, error) {
	if d.CodeIntel == nil {
		return nil, errors.New("toolset.NewResolver: CodeIntel is nil")
	}
	if d.ReadTracker == nil {
		return nil, errors.New("toolset.NewResolver: ReadTracker is nil")
	}
	resolver := &Resolver{
		defaultWorkdir:  d.DefaultWorkdir,
		defaultModel:    d.DefaultModel,
		skillsGlobalDir: d.SkillsGlobalDir,
		skillUsage:      d.SkillUsage,
		online:          slices.Clone(d.Online),
		a2a:             slices.Clone(d.A2A),
		lsp:             slices.Clone(d.LSP),
		shell:           slices.Clone(d.Shell),
		staticTools: []staticToolSpec{
			{tool: d.AskUser, audience: toolAudienceBoth, placement: toolAfterCodebase},
			{tool: d.EnterPlan, audience: toolAudienceCoding, placement: toolAfterCodebase},
			{tool: d.ExitPlan, audience: toolAudienceCoding, placement: toolAfterCodebase},
			{tool: d.Plan, audience: toolAudienceCoding, placement: toolAfterSkill},
			{tool: d.Schedule, audience: toolAudienceCoding, placement: toolCodingTail, deferred: true},
			{tool: d.ToolResult, audience: toolAudienceBoth, placement: toolAfterSkill},
			{tool: d.MemorySearch, audience: toolAudienceBoth, placement: toolAfterSkill, deferred: true},
			{tool: d.SessionSearch, audience: toolAudienceBoth, placement: toolAfterSkill, deferred: true},
			{tool: d.SkillPropose, audience: toolAudienceCoding, placement: toolCodingTail, deferred: true},
			{tool: d.GoalGet, audience: toolAudienceCoding, placement: toolCodingTail},
			{tool: d.GoalReport, audience: toolAudienceCoding, placement: toolCodingTail, requiresActiveGoal: true},
		},
		goalActive:      d.GoalActive,
		codeIntel:       d.CodeIntel,
		readTracker:     d.ReadTracker,
		pathLocker:      newPathLocker(),
		codebaseIndex:   d.CodebaseIndex,
		downloadAllow:   d.DownloadAllow,
		mcpToolDisabled: d.MCPToolDisabled,
	}
	return resolver, nil
}

func (r *Resolver) appendStaticTools(ctx context.Context, into *resolvedToolset, placement toolPlacement, role string) error {
	for _, spec := range r.staticTools {
		if spec.tool == nil || spec.placement != placement || !spec.audience.includes(role) {
			continue
		}
		if spec.requiresActiveGoal {
			if r.goalActive == nil {
				continue
			}
			active, err := r.goalActive(ctx, executionctx.SessionID(ctx))
			if err != nil {
				return fmt.Errorf("toolset: resolve report_goal_outcome availability: %w", err)
			}
			if !active {
				continue
			}
		}
		if spec.deferred {
			into.deferTools(spec.tool)
		} else {
			into.direct(spec.tool)
		}
	}
	return nil
}

// UseTaskTool installs the task delegation tool for both execution roles. The
// agent engine builds this tool after it exists because the tool starts child
// processes through that engine.
func (r *Resolver) UseTaskTool(tool toolcontract.Tool) {
	r.lateMu.Lock()
	defer r.lateMu.Unlock()
	r.task = tool
}

func (r *Resolver) taskTool() toolcontract.Tool {
	r.lateMu.RLock()
	defer r.lateMu.RUnlock()
	return r.task
}

// UseCreateGoalTool installs the root-only autonomous Goal entry tool after
// Bootstrap has constructed the Goal Driver over Runs. This is a narrow
// construction seam: the resolver still knows only a generic tool contract.
func (r *Resolver) UseCreateGoalTool(tool toolcontract.Tool) {
	r.lateMu.Lock()
	defer r.lateMu.Unlock()
	r.createGoal = tool
}

func (r *Resolver) createGoalTool() toolcontract.Tool {
	r.lateMu.RLock()
	defer r.lateMu.RUnlock()
	return r.createGoal
}

// mcpTools returns the current MCP tool set (nil before the first store) minus
// any tools the configured servers disable. The disabled set is read here, not
// at SetMCPTools, so it stays correct regardless of which hot-swap fired last
// (a reconnect that swaps tools vs. a configure that swaps the disabled set).
// The common case (nothing disabled) returns the stored slice unchanged — no
// per-resolution copy.
func (r *Resolver) mcpTools() []toolcontract.Tool {
	p := r.mcp.Load()
	if p == nil {
		return nil
	}
	values := *p
	if r.mcpToolDisabled == nil {
		return values
	}
	var out []toolcontract.Tool
	for i, tool := range values {
		ref, ok := mcpToolRef(tool)
		if !ok || r.mcpToolDisabled(ref) {
			if out == nil {
				out = append(make([]toolcontract.Tool, 0, len(values)-1), values[:i]...)
			}
			continue
		}
		if out != nil {
			out = append(out, tool)
		}
	}
	if out == nil {
		return values
	}
	return out
}

// SetMCPTools swaps in a freshly-built MCP tool set (boot + each reconnect).
func (r *Resolver) SetMCPTools(tools []toolcontract.Tool) {
	snapshot := slices.Clone(tools)
	r.mcp.Store(&snapshot)
}

func mcpToolRef(tool toolcontract.Tool) (mcpserver.ToolRef, bool) {
	identity, ok := tool.(mcpToolIdentity)
	if !ok {
		return mcpserver.ToolRef{}, false
	}
	server, remote := identity.MCPToolIdentity()
	if server == "" || remote == "" {
		return mcpserver.ToolRef{}, false
	}
	return mcpserver.ToolRef{Server: server, Tool: remote}, true
}

func (*Resolver) Name() string { return "coding-tools" }

func (r *Resolver) Resolve(_ context.Context, role string) (core.ToolGroup, bool, error) {
	switch role {
	case domaintool.GroupCoding, domaintool.GroupSubtask:
		return &toolGroup{resolver: r, role: role}, true, nil
	default:
		return nil, false, nil // unknown role — the runtime skips to the next resolver
	}
}

// workdirFor reads the per-turn working directory, falling back to the
// engine default.
func (r *Resolver) workdirFor(ctx context.Context) string {
	return executionctx.CWD(ctx, r.defaultWorkdir)
}

func (r *Resolver) workdirTools(workdir string) workdirToolFamilies {
	return buildWorkdirTools(workdir, r.codeIntel, r.readTracker, r.downloadAllow, r.pathLocker)
}

// toolGroup resolves its tool slice lazily at Tools() time so it can read the
// per-process working directory. Both execution roles receive task delegation;
// Agent Runtime's tree-wide budget and max-child-depth remain the authorities
// for bounded recursion. Root-only product tools stay on ToolRoleCoding.
type toolGroup struct {
	resolver *Resolver
	role     string
}

func (g *toolGroup) Tools(ctx context.Context) ([]toolcontract.Tool, error) {
	workdir := g.resolver.workdirFor(ctx)
	workdirTools := g.resolver.workdirTools(workdir)
	var tools resolvedToolset
	tools.direct(workdirTools.readSearch...)
	selection := executionctx.ModelSelection(ctx, g.resolver.defaultModel)
	if useApplyPatch(selection) {
		tools.direct(workdirTools.applyPatch)
	} else {
		tools.direct(workdirTools.editWrite...)
	}
	tools.deferTools(workdirTools.download)
	tools.deferTools(g.resolver.online...)
	mcpTools := g.resolver.mcpTools()
	tools.deferTools(mcpTools...)
	tools.deferTools(g.resolver.a2a...)
	tools.deferTools(g.resolver.lsp...)
	tools.direct(g.resolver.shell...)
	// The skill tool is working-directory scoped (project skills live under
	// the turn's cwd), so it is built per resolution like filesystem tools and is
	// available to both coding and subtask roles. nil when no skills exist.
	if skillTool := skill.Build(workdir, g.resolver.skillsGlobalDir, g.resolver.skillUsage); skillTool != nil {
		tools.deferTools(skillTool)
	}
	// Built-once, session-keyed helpers (plan/result/memory/transcript search)
	// are projected from the resolver's role and placement policy.
	if err := g.resolver.appendStaticTools(ctx, &tools, toolAfterSkill, g.role); err != nil {
		return nil, err
	}
	// codebase_search (both roles): semantic code search over the turn's cwd.
	// Offered only when an embedding model is configured (Available reads the
	// live embedding role), so it appears once the user sets one — no restart.
	// Dependency failures are not treated as a missing model: failing resolution
	// keeps the runtime from advertising a false capability state.
	if index := g.resolver.codebaseIndex; index != nil {
		available, err := index.Available(ctx)
		if err != nil {
			return nil, fmt.Errorf("toolset: resolve codebase_search availability: %w", err)
		}
		if available {
			codebaseSearch, err := codebasesearch.New(index)
			if err != nil {
				return nil, fmt.Errorf("toolset: resolve codebase_search: %w", err)
			}
			tools.deferTools(codebaseSearch)
		}
	}
	// Both roles can ask the user and leave plan mode. A child question parks
	// through the same nested suspension tree as a child approval.
	if err := g.resolver.appendStaticTools(ctx, &tools, toolAfterCodebase, g.role); err != nil {
		return nil, err
	}
	if task := g.resolver.taskTool(); task != nil {
		tools.direct(task)
	}
	if g.role == domaintool.GroupCoding {
		// Goal lifecycle entry is late-bound because its application Driver owns
		// Runs, while the resolver itself was needed to build the Agent executor.
		// Keep only the generic tool at this seam; no Driver or runtime state enters
		// the Agent module.
		if createGoal := g.resolver.createGoalTool(); createGoal != nil {
			tools.direct(createGoal)
		}
		// The remaining schedule, authoring, and Goal state capabilities are
		// product-root operations rather than generic child execution tools.
		if err := g.resolver.appendStaticTools(ctx, &tools, toolCodingTail, g.role); err != nil {
			return nil, err
		}
	}
	// search_tools is the sole model-facing entry to every capability withheld
	// from the initial manifest. The tools themselves remain in the same Run
	// registry, so promotion changes visibility rather than execution authority.
	search, err := toolsearch.New(tools.deferred)
	if err != nil {
		return nil, fmt.Errorf("toolset: resolve search_tools: %w", err)
	}
	if search != nil {
		tools.direct(search)
	}
	return tools.all, nil
}
