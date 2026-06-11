package toolset

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/codeintel"
	"github.com/Tangerg/lynx/tools/bash"
	"github.com/Tangerg/lynx/tools/fs"
	"github.com/Tangerg/lynx/tools/httpreq"
	"github.com/Tangerg/lynx/tools/webfetch"
	"github.com/Tangerg/lynx/tools/webfetch/jina"
	"github.com/Tangerg/lynx/tools/websearch"
	"github.com/Tangerg/lynx/tools/websearch/tavily"
)

// ToolRoleCoding is the role the main chat agent declares: the full
// coding tool set PLUS the `task` delegation tool.
//
// ToolRoleSubtask is the role the sub-agent behind `task` declares: the
// SAME coding tools but WITHOUT `task` itself, so a delegated subtask
// can't recurse into another delegation. The two-role split is the
// recursion guard.
const (
	ToolRoleCoding  = "coding"
	ToolRoleSubtask = "subtask"
)

// CwdBindingKey is the blackboard key the chat action binds (protected)
// with the turn's working directory — see the [chatInput.Cwd] handling in
// buildChatAgent. [Resolver] reads it back at tool-resolution time
// so the filesystem + bash tools operate in the session's project
// directory. Binding it protected is what carries it to `task` sub-agents:
// [core.Blackboard.Spawn] copies protected entries onto the child and the
// typed-action ClearBlackboard preserves them, so a plain Set would be lost
// when the sub-agent's action clears its inherited blackboard.
const CwdBindingKey = "lyra:cwd"

// ChatModeBindingKey is the blackboard key the chat action binds (protected)
// when a turn runs tool-less (runs.start mode=chat). [toolGroup.Tools] reads
// it back and yields an empty tool set, so the turn is a plain LLM exchange.
const ChatModeBindingKey = "lyra:chat-mode"

// SessionBindingKey is the blackboard key the chat action binds (protected)
// with the turn's session id, so the read/edit guards ([ReadTracker]) can key
// file-read state per session — read in the same seam as the working directory
// (see turnSession / [CwdBindingKey]). Protected so it rides to `task`
// sub-agents and survives the snapshot/resume round trip.
const SessionBindingKey = "lyra:session"

// wrapTool returns a Tool that runs call while preserving inner's Definition
// and Metadata — the shared spine of the tool decorators (read/edit guards,
// post-edit diagnostics). A valid inner yields a valid definition, so the
// chat.NewTool error is impossible and discarded.
func wrapTool(inner chat.Tool, call func(ctx context.Context, arguments string) (string, error)) chat.Tool {
	t, _ := chat.NewTool(inner.Definition(), inner.Metadata(), call)
	return t
}

// BuildWorkdirTools instantiates the working-directory-bound coding tools —
// the five filesystem tools and bash, all anchored at workdir. These are the
// only tools whose behavior depends on the working directory, so they are
// rebuilt per resolution (cheap structs) rather than captured once. No
// credentials needed; safe to build unconditionally.
//
// write and edit are wrapped so a successful edit is type-checked by the
// code-intelligence service and any new problems are folded into the tool
// result (see withEditDiagnostics). ci may be nil — the wrap is then a no-op.
func BuildWorkdirTools(workdir string, ci *codeintel.Service, tracker *ReadTracker) []chat.Tool {
	fsExec := fs.NewLocalExecutor(workdir)
	bashExec := bash.NewLocalExecutor()
	bashExec.Dir = workdir
	return []chat.Tool{
		withReadTracking(fs.NewReadTool(fsExec), tracker, workdir),
		// write/edit: the LSP diagnostics wrap is inner (runs on the applied
		// change); the read guard is outer (gates before the change, refreshes
		// the read stamp after).
		withWriteGuard(withEditDiagnostics(fs.NewWriteTool(fsExec), ci, workdir), tracker, workdir),
		withEditGuard(withEditDiagnostics(fs.NewEditTool(fsExec), ci, workdir), tracker, workdir),
		fs.NewGlobTool(fsExec),
		fs.NewGrepTool(fsExec),
		bash.NewTool(bashExec),
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
// engine construction and captured here; the filesystem + bash tools are
// rebuilt per resolution against the working directory the resolving
// process carries on its blackboard ([CwdBindingKey]), falling back to
// defaultWorkdir. That is what lets a single engine serve many sessions —
// each running its tools in its own project directory — without a
// per-session engine.
type Resolver struct {
	defaultWorkdir  string
	skillsGlobalDir string             // user-scope skills dir; merged under each turn's project skills
	online          []chat.Tool        // working-directory-independent network tools
	a2a             []chat.Tool        // working-directory-independent remote A2A agents
	lsp             []chat.Tool        // code-intelligence tools; cwd read per-call (service keys servers by root)
	codeIntel       *codeintel.Service // backs the write/edit diagnostics wrap (rebuilt per resolution with the turn's cwd)
	readTracker     *ReadTracker       // backs the read-before-edit + stale guards on read/edit/write
	bgShell         []chat.Tool        // background-command tools (run_in_background / bash_output / kill_shell); cwd read per-call
	task            chat.Tool          // delegation tool; coding role only, nil until set
	askUser         chat.Tool          // ask_user HITL tool; coding role only, built by engine + injected

	// mcp is the working-directory-independent MCP tool set, held behind an
	// atomic pointer so a reconnect (B3b-2) can hot-swap the live set without
	// locking the per-turn resolution path: Tools() does one atomic load, the
	// reconnect does one atomic store. The model therefore always sees the
	// currently-connected servers' tools, even mid-session.
	mcp atomic.Pointer[[]chat.Tool]
}

// Deps bundles the working-directory-independent inputs the resolver captures
// at construction. The fs/bash/lsp/skill tools are rebuilt per resolution
// against the turn's cwd; the online / A2A sets and the code-intelligence
// service are built once and held.
type Deps struct {
	DefaultWorkdir  string
	SkillsGlobalDir string
	Online          []chat.Tool        // network tools (webfetch/websearch/httpreq)
	A2A             []chat.Tool        // remote A2A delegation tools
	LSP             []chat.Tool        // code-intelligence tools
	BgShell         []chat.Tool        // background-command tools
	CodeIntel       *codeintel.Service // backs the post-edit diagnostics wrap
	ReadTracker     *ReadTracker       // backs the read/edit/write guards
}

// NewResolver builds the platform-scope tool resolver from its
// working-directory-independent inputs. The `task` (delegation) and `ask_user`
// (HITL) tools are injected afterward via [Resolver.SetTask] / [Resolver.
// SetAskUser] (they need the platform / the engine's HITL contract); the MCP
// tool set is seeded + hot-swapped via [Resolver.SetMCPTools].
func NewResolver(d Deps) *Resolver {
	return &Resolver{
		defaultWorkdir:  d.DefaultWorkdir,
		skillsGlobalDir: d.SkillsGlobalDir,
		online:          d.Online,
		a2a:             d.A2A,
		lsp:             d.LSP,
		bgShell:         d.BgShell,
		codeIntel:       d.CodeIntel,
		readTracker:     d.ReadTracker,
	}
}

// SetTask injects the `task` delegation tool (coding role only) — the engine
// builds it after the platform exists (it spawns a sub-agent on the platform).
func (r *Resolver) SetTask(t chat.Tool) { r.task = t }

// SetAskUser injects the ask_user HITL tool (coding role only) — the engine
// builds it (it rides the engine's HITL interrupt contract).
func (r *Resolver) SetAskUser(t chat.Tool) { r.askUser = t }

// mcpTools returns the current MCP tool set (nil before the first store).
func (r *Resolver) mcpTools() []chat.Tool {
	if p := r.mcp.Load(); p != nil {
		return *p
	}
	return nil
}

// SetMCPTools swaps in a freshly-built MCP tool set (boot + each reconnect).
func (r *Resolver) SetMCPTools(tools []chat.Tool) {
	r.mcp.Store(&tools)
}

func (*Resolver) Name() string { return "coding-tools" }

func (r *Resolver) Resolve(_ context.Context, req core.ToolGroupRequirement) (core.ToolGroup, error) {
	switch req.Role {
	case ToolRoleCoding, ToolRoleSubtask:
		return &toolGroup{resolver: r, role: req.Role}, nil
	default:
		return nil, nil // unknown role — the runtime skips to the next resolver
	}
}

// workdirFor reads the per-turn working directory, falling back to the
// engine default.
func (r *Resolver) workdirFor(ctx context.Context) string {
	return TurnCwd(ctx, r.defaultWorkdir)
}

// TurnCwd reads the working directory the running process seeded on its
// blackboard ([CwdBindingKey]), falling back to fallback when the turn
// carried none (a sessionless smoke run, or a restored continuation
// whose snapshot predates cwd seeding). This is THE per-session-cwd
// seam: the tool resolver, the skill tool, and the system-prompt
// composition all read the same key, so everything cwd-dependent
// follows the session together.
func TurnCwd(ctx context.Context, fallback string) string {
	p := core.ProcessFrom(ctx)
	if p == nil {
		return fallback
	}
	if v, ok := p.Blackboard().Get(CwdBindingKey); ok {
		if cwd, ok := v.(string); ok && cwd != "" {
			return cwd
		}
	}
	return fallback
}

// turnSession reads the session id the chat action seeded on the blackboard
// ([SessionBindingKey]), empty when the turn carried none (a sessionless smoke
// run). The read/edit guards key per-session file-read state off it.
func turnSession(ctx context.Context) string {
	p := core.ProcessFrom(ctx)
	if p == nil {
		return ""
	}
	if v, ok := p.Blackboard().Get(SessionBindingKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// chatModeFrom reports whether the resolving process is a tool-less chat turn
// (the chat action bound [ChatModeBindingKey]). Read off the same blackboard
// seam as the working directory (see TurnCwd).
func chatModeFrom(ctx context.Context) bool {
	p := core.ProcessFrom(ctx)
	if p == nil {
		return false
	}
	v, ok := p.Blackboard().Get(ChatModeBindingKey)
	if !ok {
		return false
	}
	on, _ := v.(bool)
	return on
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
	// Tool-less chat (runs.start mode=chat): the turn bound ChatModeBindingKey,
	// so resolve no tools — the model gets a plain single-round exchange.
	if chatModeFrom(ctx) {
		return nil, nil
	}
	workdir := g.resolver.workdirFor(ctx)
	tools := BuildWorkdirTools(workdir, g.resolver.codeIntel, g.resolver.readTracker)
	tools = append(tools, g.resolver.online...)
	tools = append(tools, g.resolver.mcpTools()...)
	tools = append(tools, g.resolver.a2a...)
	tools = append(tools, g.resolver.lsp...)
	tools = append(tools, g.resolver.bgShell...)
	// The skill tool is working-directory scoped (project skills live under
	// the turn's cwd), so it is built per resolution like fs/bash and is
	// available to both coding and subtask roles. nil when no skills exist.
	if skillTool := BuildSkillTool(workdir, g.resolver.skillsGlobalDir); skillTool != nil {
		tools = append(tools, skillTool)
	}
	if g.role == ToolRoleCoding {
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
	}
	return tools, nil
}
