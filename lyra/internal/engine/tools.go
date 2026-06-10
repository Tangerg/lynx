package engine

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
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

// cwdBindingKey is the blackboard key the chat action binds (protected)
// with the turn's working directory — see the [chatInput.Cwd] handling in
// buildChatAgent. [cwdToolResolver] reads it back at tool-resolution time
// so the filesystem + bash tools operate in the session's project
// directory. Binding it protected is what carries it to `task` sub-agents:
// [core.Blackboard.Spawn] copies protected entries onto the child and the
// typed-action ClearBlackboard preserves them, so a plain Set would be lost
// when the sub-agent's action clears its inherited blackboard.
const cwdBindingKey = "lyra:cwd"

// chatModeBindingKey is the blackboard key the chat action binds (protected)
// when a turn runs tool-less (runs.start mode=chat). [cwdToolGroup.Tools] reads
// it back and yields an empty tool set, so the turn is a plain LLM exchange.
const chatModeBindingKey = "lyra:chat-mode"

// buildWorkdirTools instantiates the working-directory-bound coding tools —
// the five filesystem tools and bash, all anchored at workdir. These are the
// only tools whose behavior depends on the working directory, so they are
// rebuilt per resolution (cheap structs) rather than captured once. No
// credentials needed; safe to build unconditionally.
func buildWorkdirTools(workdir string) []chat.Tool {
	fsExec := fs.NewLocalExecutor(workdir)
	bashExec := bash.NewLocalExecutor()
	bashExec.Dir = workdir
	return []chat.Tool{
		fs.NewReadTool(fsExec),
		fs.NewWriteTool(fsExec),
		fs.NewEditTool(fsExec),
		fs.NewGlobTool(fsExec),
		fs.NewGrepTool(fsExec),
		bash.NewTool(bashExec),
	}
}

// buildOnlineTools instantiates each network-reaching tool whose
// credentials are present in online. These are working-directory
// independent, so they are built once and shared across all resolutions.
// Missing credentials silently skip the corresponding tool — explicit
// opt-in is the safety model. Returns an error only when a configured
// provider fails to build (e.g. invalid HTTP allowlist).
func buildOnlineTools(online OnlineConfig) ([]chat.Tool, error) {
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

// cwdToolResolver is the platform-scope [core.ToolGroupResolver] for the
// coding + subtask roles. The working-directory-independent tools (online
// providers, MCP servers, the `task` delegation tool) are built once at
// engine construction and captured here; the filesystem + bash tools are
// rebuilt per resolution against the working directory the resolving
// process carries on its blackboard ([cwdBindingKey]), falling back to
// defaultWorkdir. That is what lets a single engine serve many sessions —
// each running its tools in its own project directory — without a
// per-session engine.
type cwdToolResolver struct {
	defaultWorkdir  string
	skillsGlobalDir string      // user-scope skills dir; merged under each turn's project skills
	online          []chat.Tool // working-directory-independent network tools
	mcp             []chat.Tool // working-directory-independent MCP tools
	a2a             []chat.Tool // working-directory-independent remote A2A agents
	task            chat.Tool   // delegation tool; coding role only, nil until set
}

func (*cwdToolResolver) Name() string { return "coding-tools" }

func (r *cwdToolResolver) Resolve(_ context.Context, req core.ToolGroupRequirement) (core.ToolGroup, error) {
	switch req.Role {
	case ToolRoleCoding, ToolRoleSubtask:
		return &cwdToolGroup{resolver: r, role: req.Role}, nil
	default:
		return nil, nil // unknown role — the runtime skips to the next resolver
	}
}

// workdirFor reads the per-turn working directory, falling back to the
// engine default.
func (r *cwdToolResolver) workdirFor(ctx context.Context) string {
	return turnCwd(ctx, r.defaultWorkdir)
}

// turnCwd reads the working directory the running process seeded on its
// blackboard ([cwdBindingKey]), falling back to fallback when the turn
// carried none (a sessionless smoke run, or a restored continuation
// whose snapshot predates cwd seeding). This is THE per-session-cwd
// seam: the tool resolver, the skill tool, and the system-prompt
// composition all read the same key, so everything cwd-dependent
// follows the session together.
func turnCwd(ctx context.Context, fallback string) string {
	p := core.ProcessFrom(ctx)
	if p == nil {
		return fallback
	}
	if v, ok := p.Blackboard().Get(cwdBindingKey); ok {
		if cwd, ok := v.(string); ok && cwd != "" {
			return cwd
		}
	}
	return fallback
}

// chatModeFrom reports whether the resolving process is a tool-less chat turn
// (the chat action bound [chatModeBindingKey]). Read off the same blackboard
// seam as the working directory (see turnCwd).
func chatModeFrom(ctx context.Context) bool {
	p := core.ProcessFrom(ctx)
	if p == nil {
		return false
	}
	v, ok := p.Blackboard().Get(chatModeBindingKey)
	if !ok {
		return false
	}
	on, _ := v.(bool)
	return on
}

// cwdToolGroup resolves its tool slice lazily at Tools() time so it can read
// the per-process working directory. ToolRoleSubtask omits the `task` tool so
// a delegated subtask can't recurse into another delegation.
type cwdToolGroup struct {
	resolver *cwdToolResolver
	role     string
}

func (g *cwdToolGroup) Metadata() core.ToolGroupMetadata {
	return core.SimpleToolGroupMetadata{RoleText: g.role}
}

func (g *cwdToolGroup) Tools(ctx context.Context) ([]core.AgentTool, error) {
	// Tool-less chat (runs.start mode=chat): the turn bound chatModeBindingKey,
	// so resolve no tools — the model gets a plain single-round exchange.
	if chatModeFrom(ctx) {
		return nil, nil
	}
	workdir := g.resolver.workdirFor(ctx)
	tools := buildWorkdirTools(workdir)
	tools = append(tools, g.resolver.online...)
	tools = append(tools, g.resolver.mcp...)
	tools = append(tools, g.resolver.a2a...)
	// The skill tool is working-directory scoped (project skills live under
	// the turn's cwd), so it is built per resolution like fs/bash and is
	// available to both coding and subtask roles. nil when no skills exist.
	if skillTool := buildSkillTool(workdir, g.resolver.skillsGlobalDir); skillTool != nil {
		tools = append(tools, skillTool)
	}
	if g.role == ToolRoleCoding {
		// Coding role only: the `task` delegation tool (no recursion) and
		// ask_user (HITL question). Sub-agents (ToolRoleSubtask) get neither —
		// no nested delegation, and no sub-process interrupts to supervise.
		if g.resolver.task != nil {
			tools = append(tools, g.resolver.task)
		}
		tools = append(tools, newAskUserTool())
	}
	return tools, nil
}
