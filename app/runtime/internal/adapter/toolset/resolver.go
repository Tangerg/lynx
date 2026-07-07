package toolset

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/codebasesearch"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/shell"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turnctx"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/tools/fs"
	"github.com/Tangerg/lynx/tools/httpreq"
	"github.com/Tangerg/lynx/tools/webfetch"
	"github.com/Tangerg/lynx/tools/webfetch/jina"
	"github.com/Tangerg/lynx/tools/websearch"
	"github.com/Tangerg/lynx/tools/websearch/tavily"
)

// The per-turn blackboard seam (cwd / session / chat-mode keys + readers) lives
// in package turnctx — the resolver, the per-tool packages, and the engine's
// prompt composition all read it inward without coupling to each other.

// wrapTool returns a Tool that runs call while preserving inner's Definition
// — the shared spine of the tool decorators (read/edit guards, post-edit
// diagnostics). It also forwards inner's optional tool-loop declarations so a
// keyed file tool's per-path conflict class and return-direct policy survive
// the whole decorator stack.
func wrapTool(inner chat.Tool, call func(ctx context.Context, arguments string) (string, error)) chat.Tool {
	return &decoratedTool{inner: inner, call: call}
}

// decoratedTool is the backing type for [wrapTool]: it overrides Call while
// delegating Definition plus optional tool-loop declarations to the wrapped
// tool, so a stack of decorators preserves the inner tool's full contract.
type decoratedTool struct {
	inner chat.Tool
	call  func(ctx context.Context, arguments string) (string, error)
}

func (d *decoratedTool) Definition() chat.ToolDefinition { return d.inner.Definition() }

func (d *decoratedTool) Call(ctx context.Context, arguments string) (string, error) {
	return d.call(ctx, arguments)
}

// ConcurrencyKey forwards the wrapped tool's concurrency declaration (matched
// structurally so this package needn't import the loop driver), so a keyed file
// tool keeps its per-path key through the decorator stack. A wrapped tool that
// declares nothing is exclusive (concurrent=false).
func (d *decoratedTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if c, ok := d.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	}); ok {
		return c.ConcurrencyKey(arguments)
	}
	return "", false
}

func (d *decoratedTool) ReturnsDirect() bool {
	if direct, ok := d.inner.(interface{ ReturnsDirect() bool }); ok {
		return direct.ReturnsDirect()
	}
	return false
}

// BuildWorkdirTools instantiates the working-directory-bound filesystem tools,
// all anchored at workdir. These are the only tools whose behavior depends on
// the working directory, so they are rebuilt per resolution (cheap structs)
// rather than captured once. No credentials needed; safe to build
// unconditionally. (the shell tool is built over the shared exec.Shells in
// shell.Build, not here — it reads cwd per call like shell_output.)
//
// write and edit are wrapped so a successful edit is type-checked by the
// code-intelligence analyzer and any new problems are folded into the tool
// result (see withEditDiagnostics). ci may be nil — the wrap is then a no-op.
func BuildWorkdirTools(workdir string, ci *codeintel.Analyzer, tracker *editguard.Tracker) []chat.Tool {
	fsExec := fs.NewLocalExecutor(workdir)

	// write/edit guard stack, innermost → outermost: diagnostics (type-check
	// the applied change) → read/staleness guard (gate before the change,
	// refresh the read stamp after) → per-path lock (serialize concurrent
	// writes to the same file; read-before + write stay atomic) → path guard
	// (refuse writes into protected dirs like .git — checked first). One locker
	// is shared by write + edit so they serialize against each other per path.
	locker := newPathLocker()
	write := withPathGuard(withPathLock(withWriteGuard(withEditDiagnostics(fs.NewWriteTool(fsExec), ci, workdir), tracker, workdir), locker, workdir), workdir)
	edit := withPathGuard(withPathLock(withEditGuard(withEditDiagnostics(fs.NewEditTool(fsExec), ci, workdir), tracker, workdir), locker, workdir), workdir)

	return []chat.Tool{
		withReadTracking(fs.NewReadTool(fsExec), tracker, workdir),
		write,
		edit,
		fs.NewGlobTool(fsExec),
		fs.NewGrepTool(fsExec),
	}
}

// OnlineConfig groups the credentials network-reaching tools need (webfetch /
// websearch / httpreq). Empty fields disable the corresponding tool — no tool
// is registered without explicit opt-in, so an offline-only install makes no
// surprise outbound calls. The config layer stores it directly (no bridge
// mapping); engine.Config aliases it.
type OnlineConfig struct {
	// JinaAPIKey enables the webfetch tool backed by Jina Reader.
	JinaAPIKey string

	// TavilyAPIKey enables the websearch tool backed by Tavily.
	TavilyAPIKey string

	// HTTPAllowedHosts enables the httpreq tool. Pass an explicit allowlist
	// (e.g. ["api.github.com", "*.openai.com"]) — empty keeps the tool disabled
	// so the LLM can't reach arbitrary internal endpoints.
	HTTPAllowedHosts []string
}

// BuildOnlineTools instantiates each network-reaching tool whose
// credentials are present in online. These are working-directory
// independent, so they are built once and shared across all resolutions.
// Missing credentials silently skip the corresponding tool — explicit
// opt-in is the safety model. Returns an error only when a configured
// provider fails to build (e.g. invalid HTTP allowlist).
func BuildOnlineTools(online OnlineConfig) ([]chat.Tool, error) {
	var (
		out []chat.Tool
		err error
	)

	out, err = appendIfBuilt(out, online.JinaAPIKey != "", "webfetch (jina)", func() (chat.Tool, error) {
		client, clientErr := jina.NewClient(&jina.Config{APIKey: online.JinaAPIKey})
		if clientErr != nil {
			return nil, clientErr
		}
		return webfetch.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendIfBuilt(out, online.TavilyAPIKey != "", "websearch (tavily)", func() (chat.Tool, error) {
		client, clientErr := tavily.NewClient(&tavily.Config{APIKey: online.TavilyAPIKey})
		if clientErr != nil {
			return nil, clientErr
		}
		return websearch.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendIfBuilt(out, len(online.HTTPAllowedHosts) > 0, "httpreq", func() (chat.Tool, error) {
		client, clientErr := httpreq.NewClient(httpreq.Config{AllowedHosts: online.HTTPAllowedHosts})
		if clientErr != nil {
			return nil, clientErr
		}
		return httpreq.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// appendIfBuilt is the conditional-tool-registration helper. When
// cond is false it returns tools unchanged (the credentials weren't
// supplied so the tool stays disabled — explicit opt-in is the
// safety model). When cond is true it runs build(); a non-nil
// error is wrapped with the label so the caller can tell which
// provider mis-configured.
func appendIfBuilt(tools []chat.Tool, cond bool, label string, build func() (chat.Tool, error)) ([]chat.Tool, error) {
	if !cond {
		return tools, nil
	}
	tool, err := build()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return append(tools, tool), nil
}

// Resolver is the platform-scope [core.ToolGroupResolver] for the
// coding + subtask roles. The working-directory-independent tools (online
// providers, MCP servers, the `task` delegation tool) are built once at
// engine construction and captured here; the filesystem + shell tools are
// rebuilt per resolution against the working directory the resolving
// process carries on its blackboard ([CwdBindingKey]), falling back to
// defaultWorkdir. That is what lets a single engine serve many sessions —
// each running its tools in its own project directory — without a
// per-session engine.
type Resolver struct {
	defaultWorkdir  string
	skillsGlobalDir string              // user-scope skills dir; merged under each turn's project skills
	online          []chat.Tool         // working-directory-independent network tools
	a2a             []chat.Tool         // working-directory-independent remote A2A agents
	lsp             []chat.Tool         // code-intelligence tools; cwd read per-call (analyzer keys servers by root)
	codeIntel       *codeintel.Analyzer // backs the write/edit diagnostics wrap (rebuilt per resolution with the turn's cwd)
	readTracker     *editguard.Tracker  // backs the read-before-edit + stale guards on read/edit/write
	shell           []chat.Tool         // shell tools (shell / shell_output / shell_kill) over the exec.Shells; cwd read per-call
	task            chat.Tool           // delegation tool; coding role only, nil until set
	askUser         chat.Tool           // ask_user HITL tool; coding role only (askuser.New, via Deps)
	exitPlan        chat.Tool           // exit_plan_mode HITL tool; coding role only (exitplan.New, via Deps); nil when no approval svc
	todo            chat.Tool           // todo_write task-list tool; both roles, nil when no todo store

	// codebaseIndex backs codebase_search (both roles). Held as the index (not
	// a pre-built tool) so Tools() can gate inclusion on Available() per turn —
	// the embedding model can be configured after construction. nil → no tool.
	codebaseIndex codebaseindex.Index

	// mcp is the working-directory-independent MCP tool set, held behind an
	// atomic pointer so a reconnect (B3b-2) can hot-swap the live set without
	// locking the per-turn resolution path: Tools() does one atomic load, the
	// reconnect does one atomic store. The model therefore always sees the
	// currently-connected servers' tools, even mid-session.
	mcp atomic.Pointer[[]chat.Tool]

	// mcpDisabled returns the model-facing names ("<server>_<tool>") the
	// configured servers hide from the model — a per-server blacklist the
	// runtime recomputes on every registry change. Read per resolution (not
	// folded into SetMCPTools) so it stays correct under the two independent
	// hot-swaps: the live tool set (reconnect) and the disabled set (configure).
	// nil — or an empty set — means no filtering. The set is owned upstream and
	// only read here, never mutated.
	mcpDisabled func() map[string]struct{}
}

// Deps bundles the working-directory-independent inputs the resolver captures
// at construction. The fs/shell/lsp/skill tools are rebuilt per resolution
// against the turn's cwd; the online / A2A sets and the code-intelligence
// analyzer are built once and held.
type Deps struct {
	DefaultWorkdir  string
	SkillsGlobalDir string
	Online          []chat.Tool         // network tools (webfetch/websearch/httpreq)
	A2A             []chat.Tool         // remote A2A delegation tools
	LSP             []chat.Tool         // code-intelligence tools
	Shell           []chat.Tool         // shell tools (shell / shell_output / shell_kill)
	AskUser         chat.Tool           // ask_user HITL tool (coding role only)
	ExitPlan        chat.Tool           // exit_plan_mode HITL tool (coding role only); nil → omitted
	Todo            chat.Tool           // todo_write task-list tool (both roles); nil → omitted
	CodeIntel       *codeintel.Analyzer // backs the post-edit diagnostics wrap
	ReadTracker     *editguard.Tracker  // backs the read/edit/write guards
	CodebaseIndex   codebaseindex.Index // backs codebase_search (both roles); nil → omitted

	// MCPDisabled returns the model-facing MCP tool names the configured servers
	// hide from the model (per-server blacklist; nil → no filtering). Read per
	// resolution; see [Resolver.mcpDisabled].
	MCPDisabled func() map[string]struct{}
}

// NewResolver builds the platform-scope tool resolver from its
// working-directory-independent inputs. The `task` (delegation) and `ask_user`
// (HITL) tools are injected afterward via [Resolver.SetTask] / [Resolver.
// SetAskUser] (they need the platform / the engine's HITL contract); the MCP
// tool set is seeded + hot-swapped via [Resolver.SetMCPTools].
func NewResolver(d Deps) *Resolver {
	shellTools := d.Shell
	if shellTools == nil {
		// Bare resolver (a unit-test engine with no injected tool environment):
		// own a private exec.Shells so the shell tool and its background companions are
		// still available. The production path injects shell tools built over the
		// shared shell set whose KillAll is a shutdown closer (toolset.Build); this
		// private shell set has no closer, fine for a process-lifetime test engine.
		shellTools = shell.Build(exec.NewShells(), d.DefaultWorkdir)
	}
	return &Resolver{
		defaultWorkdir:  d.DefaultWorkdir,
		skillsGlobalDir: d.SkillsGlobalDir,
		online:          d.Online,
		a2a:             d.A2A,
		lsp:             d.LSP,
		shell:           shellTools,
		askUser:         d.AskUser,
		exitPlan:        d.ExitPlan,
		todo:            d.Todo,
		codeIntel:       d.CodeIntel,
		readTracker:     d.ReadTracker,
		codebaseIndex:   d.CodebaseIndex,
		mcpDisabled:     d.MCPDisabled,
	}
}

// SetTask injects the `task` delegation tool (coding role only) — the engine
// builds it after the platform exists (it spawns a sub-agent on the platform).
func (r *Resolver) SetTask(t chat.Tool) { r.task = t }

// mcpTools returns the current MCP tool set (nil before the first store) minus
// any tools the configured servers disable. The disabled set is read here, not
// at SetMCPTools, so it stays correct regardless of which hot-swap fired last
// (a reconnect that swaps tools vs. a configure that swaps the disabled set).
// The common case (nothing disabled) returns the stored slice unchanged — no
// per-resolution copy.
func (r *Resolver) mcpTools() []chat.Tool {
	p := r.mcp.Load()
	if p == nil {
		return nil
	}
	tools := *p
	if r.mcpDisabled == nil {
		return tools
	}
	disabled := r.mcpDisabled()
	if len(disabled) == 0 {
		return tools
	}
	out := make([]chat.Tool, 0, len(tools))
	for _, t := range tools {
		if _, hide := disabled[t.Definition().Name]; hide {
			continue
		}
		out = append(out, t)
	}
	return out
}

// SetMCPTools swaps in a freshly-built MCP tool set (boot + each reconnect).
func (r *Resolver) SetMCPTools(tools []chat.Tool) {
	r.mcp.Store(&tools)
}

func (*Resolver) Name() string { return "coding-tools" }

func (r *Resolver) Resolve(_ context.Context, req core.ToolGroupRequirement) (core.ToolGroup, error) {
	switch req.Role {
	case toolport.ToolRoleCoding, toolport.ToolRoleSubtask:
		return &toolGroup{resolver: r, role: req.Role}, nil
	default:
		return nil, nil // unknown role — the runtime skips to the next resolver
	}
}

// workdirFor reads the per-turn working directory, falling back to the
// engine default.
func (r *Resolver) workdirFor(ctx context.Context) string {
	return turnctx.TurnCwd(ctx, r.defaultWorkdir)
}

// toolGroup resolves its tool slice lazily at Tools() time so it can read
// the per-process working directory. ToolRoleSubtask omits the `task` tool so
// a delegated subtask can't recurse into another delegation.
type toolGroup struct {
	resolver *Resolver
	role     string
}

func (g *toolGroup) Metadata() core.ToolGroupMetadata {
	return core.SimpleToolGroupMetadata{RoleText: g.role}
}

func (g *toolGroup) Tools(ctx context.Context) ([]core.AgentTool, error) {
	workdir := g.resolver.workdirFor(ctx)
	tools := BuildWorkdirTools(workdir, g.resolver.codeIntel, g.resolver.readTracker)
	tools = append(tools, g.resolver.online...)
	tools = append(tools, g.resolver.mcpTools()...)
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
	// codebase_search (both roles): semantic code search over the turn's cwd.
	// Offered only when an embedding model is configured (Available reads the
	// live embedding role), so it appears once the user sets one — no restart.
	if index := g.resolver.codebaseIndex; index != nil && index.Available(ctx) {
		tools = append(tools, codebasesearch.New(index))
	}
	if g.role == toolport.ToolRoleCoding {
		// Coding role only: the `task` delegation tool (no recursion) and
		// ask_user (HITL question). Both are injected by the engine (they need
		// the platform / the HITL contract). Sub-agents (ToolRoleSubtask) get
		// neither — no nested delegation, no sub-process interrupts to supervise.
		if g.resolver.task != nil {
			tools = append(tools, g.resolver.task)
		}
		if g.resolver.askUser != nil {
			tools = append(tools, g.resolver.askUser)
		}
		if g.resolver.exitPlan != nil {
			tools = append(tools, g.resolver.exitPlan)
		}
	}
	return tools, nil
}
