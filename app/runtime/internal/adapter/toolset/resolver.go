package toolset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// The per-Run application-context seam (cwd, session, isolation, goal incarnation)
// lives in package executionctx — the resolver, per-tool packages, and prompt
// composition all read it inward without coupling to each other.

// Resolver is the engine-scope [core.ToolGroupResolver] for the root and
// delegated roles. The working-directory-independent tools (online
// providers, MCP servers, the `delegate_task` tool) are built once at
// engine construction and captured here; filesystem and skill tools are
// rebuilt per resolution, while shell and LSP tools read the resolving Run's
// application context per call. Both paths fall back to the configured default
// CWD. That is
// what lets a single engine serve many sessions —
// each running its tools in its own project directory — without a
// per-session engine.
type Resolver struct {
	lateMu sync.RWMutex

	defaultCWD    string
	skillsUserDir string                     // user-scope skills dir; merged under each Run's project skills
	skillUsage    builtin.SkillUsageRecorder // records skill loads for the idle-lifecycle curator; nil → off
	online        []toolcontract.Tool        // working-directory-independent network tools
	a2a           []toolcontract.Tool        // working-directory-independent remote A2A agents
	lsp           []toolcontract.Tool        // code-intelligence tools; cwd read per-call (analyzer keys servers by root)
	codeIntel     *codeintel.Analyzer        // backs post-patch diagnostics (rebuilt per resolution with the Run's cwd)
	readTracker   *readTracker               // backs the read-before-patch and stale-read guards
	pathLocker    *pathLocker                // serializes same-path fs calls across every concurrent Run resolution
	shell         []toolcontract.Tool        // shell tools (shell / read_shell_output / stop_shell) over the exec.Shells; cwd read per-call
	createGoal    toolcontract.Tool          // root-only Goal entry tool; nil until the Goal Driver exists
	staticSpecs   []staticSpec               // built-once capabilities with one role/placement policy for Run manifests

	// mcp is the working-directory-independent MCP tool set, held behind an
	// atomic pointer so a reconnect (B3b-2) can hot-swap the live set without
	// locking the per-Run resolution path: Tools() does one atomic load, the
	// reconnect does one atomic store. The model therefore always sees the
	// currently-connected servers' tools, even mid-session.
	mcp atomic.Pointer[[]toolcontract.Tool]

	// mcpToolDisabled reads the current domain policy per resolution so registry
	// changes and live-tool reconnects remain independent hot swaps.
	mcpToolDisabled func(mcpserver.ToolRef) bool
}

type audience uint8

const (
	audienceBoth audience = iota
	audienceRoot
)

func (a audience) includes(role string) bool {
	return a == audienceBoth || role == domaintool.GroupRoot
}

type placement uint8

const (
	afterSkill placement = iota
	interactionTail
	rootTail
)

// staticSpec is the policy record for one built-once per-Run capability. A Run
// consumes entries in its placement and audience, with Goal-only capabilities
// keyed by the immutable incarnation stamped at Run admission.
type staticSpec struct {
	tool            toolcontract.Tool
	audience        audience
	placement       placement
	deferred        bool
	requiresGoalRun bool
}

// resolverDeps bundles the working-directory-independent inputs the resolver captures
// at construction. Filesystem and skill tools are rebuilt per resolution;
// shell and LSP tools are built once but read the Run's cwd per call. Online,
// A2A, and code-intelligence capabilities are also built once and held.
type resolverDeps struct {
	DefaultCWD         string
	SkillsUserDir      string
	SkillUsage         builtin.SkillUsageRecorder
	Online             []toolcontract.Tool // network tools (webfetch/websearch/httpreq)
	A2A                []toolcontract.Tool // remote A2A delegation tools
	LSP                []toolcontract.Tool // code-intelligence tools
	Shell              []toolcontract.Tool // shell tools (shell / read_shell_output / stop_shell); nil means omitted
	AskUser            toolcontract.Tool   // ask_user HITL tool (both roles)
	EnterPlan          toolcontract.Tool   // enter_plan_mode (root role only); nil → omitted
	ExitPlan           toolcontract.Tool   // exit_plan_mode (root role only); nil → omitted
	Plan               toolcontract.Tool   // set_plan execution-plan tool (root role only); nil → omitted
	ScheduleTools      []toolcontract.Tool // schedule management tools (root role only); nil → omitted
	ToolResult         toolcontract.Tool   // read_tool_result offloaded-output reader (both roles); nil → omitted
	AgentMemorySearch  toolcontract.Tool   // search_memory agent-memory reader (both roles); nil → omitted
	ConversationSearch toolcontract.Tool   // search_conversations past-transcript reader (both roles); nil → omitted
	GoalGet            toolcontract.Tool   // get_goal state reader (root role only); nil → omitted
	GoalReport         toolcontract.Tool   // report_goal_outcome loop signal (Goal-owned root Runs only); nil → omitted
	ProposeSkill       toolcontract.Tool   // propose_skill pending submission (root role only); nil → omitted
	CodeIntel          *codeintel.Analyzer // backs post-mutation diagnostics
	ReadTracker        *readTracker        // backs the read-before-patch and stale-read guards
	// MCPToolDisabled reports whether an identified MCP tool is hidden.
	MCPToolDisabled func(mcpserver.ToolRef) bool
}

type mcpToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

// newResolver builds the Runtime-scoped Tool resolver from its
// working-directory-independent inputs. The create_goal entry Tool is injected
// through an explicit seam after its cyclic application owner exists; the MCP
// Tool set is seeded and hot-swapped via [Resolver.SetMCPTools].
func newResolver(d resolverDeps) (*Resolver, error) {
	if d.CodeIntel == nil {
		return nil, errors.New("toolset: resolver code intelligence is nil")
	}
	if d.ReadTracker == nil {
		return nil, errors.New("toolset: resolver read tracker is nil")
	}
	resolver := &Resolver{
		defaultCWD:    d.DefaultCWD,
		skillsUserDir: d.SkillsUserDir,
		skillUsage:    d.SkillUsage,
		online:        slices.Clone(d.Online),
		a2a:           slices.Clone(d.A2A),
		lsp:           slices.Clone(d.LSP),
		shell:         slices.Clone(d.Shell),
		staticSpecs: []staticSpec{
			{tool: d.AskUser, audience: audienceBoth, placement: interactionTail},
			{tool: d.EnterPlan, audience: audienceRoot, placement: interactionTail},
			{tool: d.ExitPlan, audience: audienceRoot, placement: interactionTail},
			{tool: d.Plan, audience: audienceRoot, placement: afterSkill},
			{tool: d.ToolResult, audience: audienceBoth, placement: afterSkill},
			{tool: d.AgentMemorySearch, audience: audienceBoth, placement: afterSkill, deferred: true},
			{tool: d.ConversationSearch, audience: audienceBoth, placement: afterSkill, deferred: true},
			{tool: d.ProposeSkill, audience: audienceRoot, placement: afterSkill, deferred: true},
			{tool: d.GoalGet, audience: audienceRoot, placement: rootTail},
			{tool: d.GoalReport, audience: audienceRoot, placement: rootTail, requiresGoalRun: true},
		},
		codeIntel:       d.CodeIntel,
		readTracker:     d.ReadTracker,
		pathLocker:      newPathLocker(),
		mcpToolDisabled: d.MCPToolDisabled,
	}
	for _, scheduleTool := range d.ScheduleTools {
		resolver.staticSpecs = append(resolver.staticSpecs, staticSpec{
			tool: scheduleTool, audience: audienceRoot, placement: rootTail, deferred: true,
		})
	}
	return resolver, nil
}

func (r *Resolver) appendStatic(ctx context.Context, into *manifestBuilder, at placement, role string) error {
	for _, spec := range r.staticSpecs {
		if spec.tool == nil || spec.placement != at || !spec.audience.includes(role) {
			continue
		}
		if spec.requiresGoalRun {
			if _, owned := executionctx.GoalIncarnationID(ctx); !owned {
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

// UseCreateGoalTool installs the root-only autonomous Goal entry tool after
// the Goal Driver has been constructed over Runs. This is a narrow
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

// cwdFor reads the per-Run working directory, falling back to the
// engine default.
func (r *Resolver) cwdFor(ctx context.Context) string {
	return executionctx.CWD(ctx, r.defaultCWD)
}

func (r *Resolver) toolsForCWD(cwd string) cwdTools {
	return buildCWDTools(cwd, r.codeIntel, r.readTracker, r.pathLocker)
}

// Manifest resolves one role's frozen, framework-neutral Tool visibility. It
// is the integration surface for execution strategies that distinguish initial
// visibility from deferred executable authority without importing this package
// into their framework.
func (r *Resolver) Manifest(ctx context.Context, role string) (Manifest, error) {
	resolved, err := r.resolve(ctx, role)
	if err != nil {
		return Manifest{}, err
	}
	return resolved.manifest(), nil
}

func (r *Resolver) resolve(ctx context.Context, role string) (manifestBuilder, error) {
	if role != domaintool.GroupRoot && role != domaintool.GroupDelegated {
		return manifestBuilder{}, fmt.Errorf("toolset: unsupported tool role %q", role)
	}
	cwd := r.cwdFor(ctx)
	localTools := r.toolsForCWD(cwd)
	var tools manifestBuilder
	tools.direct(localTools.readSearch...)
	tools.direct(localTools.applyPatch)
	tools.deferTools(r.online...)
	mcpTools := r.mcpTools()
	tools.deferTools(mcpTools...)
	tools.deferTools(r.a2a...)
	tools.deferTools(r.lsp...)
	tools.direct(r.shell...)
	// Skill tools are working-directory scoped (project skills live under the
	// Run's cwd), so they are built per resolution like filesystem tools and are
	// available to both root and delegated roles. No tools when no skills exist.
	skillTools, err := builtin.BuildReaders(cwd, r.skillsUserDir, r.skillUsage)
	if err != nil {
		return manifestBuilder{}, fmt.Errorf("toolset: resolve skill tools: %w", err)
	}
	tools.deferTools(skillTools...)
	// Built-once, session-keyed helpers (plan/result/memory/transcript search)
	// are projected from the resolver's role and placement policy.
	if err := r.appendStatic(ctx, &tools, afterSkill, role); err != nil {
		return manifestBuilder{}, err
	}
	// Both roles can ask the user; Plan-mode controls in this placement remain
	// root-only. A child question waits at the same durable tree boundary as a
	// child approval.
	if err := r.appendStatic(ctx, &tools, interactionTail, role); err != nil {
		return manifestBuilder{}, err
	}
	if role == domaintool.GroupRoot {
		// Goal lifecycle entry is late-bound because its application Driver owns
		// Runs, while the resolver itself was needed to build the Agent executor.
		// Keep only the generic tool at this seam; no Driver or runtime state enters
		// the Agent module.
		if createGoal := r.createGoalTool(); createGoal != nil {
			tools.direct(createGoal)
		}
		// The remaining schedule and Goal state capabilities are
		// product-root operations rather than generic child execution tools.
		if err := r.appendStatic(ctx, &tools, rootTail, role); err != nil {
			return manifestBuilder{}, err
		}
	}
	// search_tools is the sole model-facing entry to every capability withheld
	// from the initial manifest. The tools themselves remain in the same Run
	// registry, so promotion changes visibility rather than execution authority.
	search, err := NewDiscovery(tools.deferred)
	if err != nil {
		return manifestBuilder{}, fmt.Errorf("toolset: resolve search_tools: %w", err)
	}
	if search != nil {
		tools.direct(search)
	}
	return tools, nil
}
