package toolset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/codebasesearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/shell"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/toolsearch"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/tools"
	"github.com/Tangerg/lynx/tools/httpreq"
)

// The per-turn blackboard seam (cwd / session / chat-mode keys + readers) lives
// in package turnctx — the resolver, the per-tool packages, and the engine's
// prompt composition all read it inward without coupling to each other.

// Resolver is the engine-scope [core.ToolGroupResolver] for the
// coding + subtask roles. The working-directory-independent tools (online
// providers, MCP servers, the `task` delegation tool) are built once at
// engine construction and captured here; the filesystem + shell tools are
// rebuilt per resolution against the working directory the resolving
// process carries on its blackboard ([CwdBindingKey]), falling back to
// defaultWorkdir. That is what lets a single engine serve many sessions —
// each running its tools in its own project directory — without a
// per-session engine.
type Resolver struct {
	catalogMu sync.RWMutex
	catalog   []tools.Tool

	defaultWorkdir  string
	skillsGlobalDir string              // user-scope skills dir; merged under each turn's project skills
	online          []tools.Tool        // working-directory-independent network tools
	a2a             []tools.Tool        // working-directory-independent remote A2A agents
	lsp             []tools.Tool        // code-intelligence tools; cwd read per-call (analyzer keys servers by root)
	codeIntel       *codeintel.Analyzer // backs the write/edit diagnostics wrap (rebuilt per resolution with the turn's cwd)
	readTracker     *editguard.Tracker  // backs the read-before-edit + stale guards on read/edit/write
	pathLocker      *pathLocker         // serializes same-path fs calls across every concurrent turn resolution
	shell           []tools.Tool        // shell tools (shell / shell_output / shell_kill) over the exec.Shells; cwd read per-call
	task            tools.Tool          // delegation tool; coding role only, nil until set
	askUser         tools.Tool          // ask_user HITL tool; coding role only (askuser.New, via Deps)
	exitPlan        tools.Tool          // exit_plan_mode HITL tool; coding role only (exitplan.New, via dependencies); nil without an approval policy
	todo            tools.Tool          // todo_write task-list tool; both roles, nil when no todo store
	schedule        tools.Tool          // schedule management op-tool; coding role only, nil when no registry
	toolResult      tools.Tool          // read_tool_result offloaded-output reader; both roles, nil when eviction is off
	skillPropose    tools.Tool          // propose_skill authoring tool; coding role only, nil when authoring is off

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
	mcp atomic.Pointer[[]tools.Tool]

	// mcpToolDisabled reads the current domain policy per resolution so registry
	// changes and live-tool reconnects remain independent hot swaps.
	mcpToolDisabled func(mcpserver.ToolRef) bool
}

// Deps bundles the working-directory-independent inputs the resolver captures
// at construction. The fs/shell/lsp/skill tools are rebuilt per resolution
// against the turn's cwd; the online / A2A sets and the code-intelligence
// analyzer are built once and held.
type Deps struct {
	DefaultWorkdir  string
	SkillsGlobalDir string
	Online          []tools.Tool        // network tools (webfetch/websearch/httpreq)
	A2A             []tools.Tool        // remote A2A delegation tools
	LSP             []tools.Tool        // code-intelligence tools
	Shell           []tools.Tool        // shell tools (shell / shell_output / shell_kill)
	AskUser         tools.Tool          // ask_user HITL tool (coding role only)
	ExitPlan        tools.Tool          // exit_plan_mode HITL tool (coding role only); nil → omitted
	Todo            tools.Tool          // todo_write task-list tool (both roles); nil → omitted
	Schedule        tools.Tool          // schedule management op-tool (coding role only); nil → omitted
	ToolResult      tools.Tool          // read_tool_result offloaded-output reader (both roles); nil → omitted
	SkillPropose    tools.Tool          // propose_skill authoring tool (coding role only); nil → omitted
	CodeIntel       *codeintel.Analyzer // backs the post-edit diagnostics wrap
	ReadTracker     *editguard.Tracker  // backs the read/edit/write guards
	CodebaseIndex   CodebaseIndex       // backs codebase_search (both roles); nil → omitted
	DownloadAllow   httpreq.Allowlist   // host allowlist gating/guarding download; empty → omitted

	// MCPToolDisabled reports whether an identified MCP tool is hidden.
	MCPToolDisabled func(mcpserver.ToolRef) bool
}

type mcpToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

// NewResolver builds the engine-scoped tool resolver from its
// working-directory-independent inputs. The `task` delegation tool is injected
// afterward via [Resolver.UseTaskTool] because it needs the agent engine; the MCP tool
// set is seeded + hot-swapped via [Resolver.SetMCPTools].
func NewResolver(d Deps) (*Resolver, error) {
	shellTools := d.Shell
	if shellTools == nil {
		// Bare resolver (a unit-test engine with no injected tool environment):
		// own a private exec.Shells so the shell tool and its background companions are
		// still available. The production path injects shell tools built over the
		// shared shell set whose KillAll is a shutdown closer (toolset.Build); this
		// private shell set has no closer, fine for a process-lifetime test engine.
		var err error
		shellTools, err = shell.Build(exec.NewShells(), d.DefaultWorkdir)
		if err != nil {
			return nil, fmt.Errorf("toolset.NewResolver: build fallback shell tools: %w", err)
		}
	}
	if d.CodeIntel == nil {
		return nil, errors.New("toolset.NewResolver: CodeIntel is nil")
	}
	if d.ReadTracker == nil {
		return nil, errors.New("toolset.NewResolver: ReadTracker is nil")
	}
	return &Resolver{
		defaultWorkdir:  d.DefaultWorkdir,
		skillsGlobalDir: d.SkillsGlobalDir,
		online:          slices.Clone(d.Online),
		a2a:             slices.Clone(d.A2A),
		lsp:             slices.Clone(d.LSP),
		shell:           slices.Clone(shellTools),
		askUser:         d.AskUser,
		exitPlan:        d.ExitPlan,
		todo:            d.Todo,
		schedule:        d.Schedule,
		toolResult:      d.ToolResult,
		skillPropose:    d.SkillPropose,
		codeIntel:       d.CodeIntel,
		readTracker:     d.ReadTracker,
		pathLocker:      newPathLocker(),
		codebaseIndex:   d.CodebaseIndex,
		downloadAllow:   d.DownloadAllow,
		mcpToolDisabled: d.MCPToolDisabled,
	}, nil
}

// UseTaskTool installs the `task` delegation tool for the coding role and the
// direct diagnostic catalog. The agent engine builds this tool after it exists
// because the tool starts child processes through that engine.
func (r *Resolver) UseTaskTool(tool tools.Tool) {
	r.catalogMu.Lock()
	defer r.catalogMu.Unlock()

	if r.task == nil {
		r.catalog = append(r.catalog, tool)
	} else {
		oldName := r.task.Definition().Name
		for index := range r.catalog {
			if r.catalog[index].Definition().Name == oldName {
				r.catalog[index] = tool
				break
			}
		}
	}
	r.task = tool
}

// Tools returns the construction-time diagnostic catalog. It is distinct from
// per-turn role resolution: metadata is rooted at the configured default
// workdir and includes capabilities that can become available at runtime.
func (r *Resolver) Tools() []tools.Tool {
	r.catalogMu.RLock()
	defer r.catalogMu.RUnlock()
	return slices.Clone(r.catalog)
}

func (r *Resolver) setCatalog(tools []tools.Tool) {
	r.catalogMu.Lock()
	defer r.catalogMu.Unlock()
	r.catalog = slices.Clone(tools)
}

func (r *Resolver) taskTool() tools.Tool {
	r.catalogMu.RLock()
	defer r.catalogMu.RUnlock()
	return r.task
}

// mcpTools returns the current MCP tool set (nil before the first store) minus
// any tools the configured servers disable. The disabled set is read here, not
// at SetMCPTools, so it stays correct regardless of which hot-swap fired last
// (a reconnect that swaps tools vs. a configure that swaps the disabled set).
// The common case (nothing disabled) returns the stored slice unchanged — no
// per-resolution copy.
func (r *Resolver) mcpTools() []tools.Tool {
	p := r.mcp.Load()
	if p == nil {
		return nil
	}
	values := *p
	if r.mcpToolDisabled == nil {
		return values
	}
	var out []tools.Tool
	for i, tool := range values {
		ref, ok := mcpToolRef(tool)
		if !ok || r.mcpToolDisabled(ref) {
			if out == nil {
				out = append(make([]tools.Tool, 0, len(values)-1), values[:i]...)
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
func (r *Resolver) SetMCPTools(tools []tools.Tool) {
	snapshot := slices.Clone(tools)
	r.mcp.Store(&snapshot)
}

func mcpToolRef(tool tools.Tool) (mcpserver.ToolRef, bool) {
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

func (r *Resolver) Resolve(_ context.Context, requirement core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
	switch requirement.Role {
	case toolport.ToolRoleCoding, toolport.ToolRoleSubtask:
		return &toolGroup{resolver: r, role: requirement.Role}, true, nil
	default:
		return nil, false, nil // unknown role — the runtime skips to the next resolver
	}
}

// workdirFor reads the per-turn working directory, falling back to the
// engine default.
func (r *Resolver) workdirFor(ctx context.Context) string {
	return turnctx.TurnCwd(ctx, r.defaultWorkdir)
}

func (r *Resolver) workdirTools(workdir string) []tools.Tool {
	return buildWorkdirTools(workdir, r.codeIntel, r.readTracker, r.downloadAllow, r.pathLocker)
}

// toolGroup resolves its tool slice lazily at Tools() time so it can read
// the per-process working directory. ToolRoleSubtask omits the `task` tool so
// a delegated subtask can't recurse into another delegation.
type toolGroup struct {
	resolver *Resolver
	role     string
}

func (g *toolGroup) Info() core.ToolGroupInfo {
	return core.ToolGroupInfo{Role: g.role}
}

func (g *toolGroup) Tools(ctx context.Context) ([]tools.Tool, error) {
	workdir := g.resolver.workdirFor(ctx)
	tools := g.resolver.workdirTools(workdir)
	tools = append(tools, g.resolver.online...)
	// MCP tools stay in the resolved set so they remain resolvable/executable,
	// but the turn manifest projection withholds them from the model's initial
	// toolset (see search_tools below): the model loads them on demand instead
	// of the prompt carrying every server's full schema each round.
	mcpTools := g.resolver.mcpTools()
	tools = append(tools, mcpTools...)
	tools = append(tools, g.resolver.a2a...)
	tools = append(tools, g.resolver.lsp...)
	tools = append(tools, g.resolver.shell...)
	// The skill tool is working-directory scoped (project skills live under
	// the turn's cwd), so it is built per resolution like fs/shell and is
	// available to both coding and subtask roles. nil when no skills exist.
	if skillTool := skill.Build(workdir, g.resolver.skillsGlobalDir); skillTool != nil {
		tools = append(tools, skillTool)
	}
	// todo_write is working-directory independent (it keys off the session id,
	// not the cwd), so it's built once and given to both roles — a delegated
	// subtask tracks its own task list the same way the main agent does.
	if g.resolver.todo != nil {
		tools = append(tools, g.resolver.todo)
	}
	// read_tool_result (both roles): reads back a tool output offloaded on
	// eviction. Session-keyed, cwd-independent so a subtask reads its own
	// offloaded results the same way the main agent does.
	if g.resolver.toolResult != nil {
		tools = append(tools, g.resolver.toolResult)
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
			tools = append(tools, codebaseSearch)
		}
	}
	// Both roles can ask the user and leave plan mode. A child question parks
	// through the same nested suspension tree as a child approval.
	if g.resolver.askUser != nil {
		tools = append(tools, g.resolver.askUser)
	}
	if g.resolver.exitPlan != nil {
		tools = append(tools, g.resolver.exitPlan)
	}
	if g.role == toolport.ToolRoleCoding {
		// Coding role only: task (no recursive delegation) and schedule (a
		// root-owned orchestration capability).
		if task := g.resolver.taskTool(); task != nil {
			tools = append(tools, task)
		}
		if g.resolver.schedule != nil {
			tools = append(tools, g.resolver.schedule)
		}
		// propose_skill curates the user's global skill library — a root-agent
		// orchestration capability, gated behind human approval; a delegated
		// subtask shouldn't author skills.
		if g.resolver.skillPropose != nil {
			tools = append(tools, g.resolver.skillPropose)
		}
	}
	// search_tools (both roles): the progressive-disclosure surface over the
	// withheld MCP tools. nil when no MCP servers are connected — then nothing
	// is deferred and the manifest is unchanged.
	if search := toolsearch.New(mcpTools); search != nil {
		tools = append(tools, search)
	}
	return tools, nil
}
