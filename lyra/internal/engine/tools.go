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

// ToolRoleCoding is the role name the chat agent declares to require
// the default coding tool group. Action bodies that opt into this
// role get every tool returned by [BuildToolSet] wired into their
// chat request.
const ToolRoleCoding = "coding"

// BuildToolSet returns the runtime's complete tool list — six
// always-on coding tools plus zero or more provider-backed online
// tools enabled by online. The six baseline tools (read / write /
// edit / glob / grep / bash) need only a filesystem root; online
// tools require explicit credentials so a misconfigured deployment
// silently runs offline rather than exposing arbitrary network
// access to the LLM.
func BuildToolSet(workdir string, online OnlineConfig) ([]chat.Tool, error) {
	tools := buildOfflineTools(workdir)

	onlineTools, err := buildOnlineTools(online)
	if err != nil {
		return nil, err
	}
	return append(tools, onlineTools...), nil
}

// buildOfflineTools instantiates the always-on coding tool set. No
// credentials needed; safe to register unconditionally.
func buildOfflineTools(workdir string) []chat.Tool {
	fsExec := fs.NewLocalExecutor(workdir)
	bashExec := bash.NewLocalExecutor()
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
// credentials are present in online. Missing credentials silently
// skip the corresponding tool — explicit opt-in is the safety
// model. Returns an error only when a configured provider fails
// to build (e.g. invalid HTTP allowlist).
func buildOnlineTools(online OnlineConfig) ([]chat.Tool, error) {
	var (
		out []chat.Tool
		err error
	)

	out, err = appendIfBuilt(out, online.JinaAPIKey != "", "webfetch (jina)", func() (chat.Tool, error) {
		client, err := jina.NewClient(&jina.Config{APIKey: online.JinaAPIKey})
		if err != nil {
			return nil, err
		}
		return webfetch.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendIfBuilt(out, online.TavilyAPIKey != "", "websearch (tavily)", func() (chat.Tool, error) {
		client, err := tavily.NewClient(&tavily.Config{APIKey: online.TavilyAPIKey})
		if err != nil {
			return nil, err
		}
		return websearch.NewTool(client)
	})
	if err != nil {
		return nil, err
	}

	out, err = appendIfBuilt(out, len(online.HTTPAllowedHosts) > 0, "httpreq", func() (chat.Tool, error) {
		client, err := httpreq.NewClient(httpreq.Config{AllowedHosts: online.HTTPAllowedHosts})
		if err != nil {
			return nil, err
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

// buildCodingResolverFromTools wires the supplied tool list behind
// [ToolRoleCoding] on a fresh [core.StaticToolGroupResolver]. The
// resolver is registered as a platform-scope extension so every
// agent (just the chat agent for now) can opt-in via
// [core.ToolRolesFor].
func buildCodingResolverFromTools(tools []chat.Tool) *core.StaticToolGroupResolver {
	resolver := core.NewStaticToolGroupResolver("coding-tools")
	resolver.Register(ToolRoleCoding, &codingToolGroup{tools: tools})
	return resolver
}

// codingToolGroup is the minimal [core.ToolGroup] wrapping our tool
// slice. It declares no permissions — the runtime accepts the group
// against any requirement (a [core.ToolGroupRequirement] with empty
// Permissions). M4 will refine this with HostAccess / InternetAccess
// labels once sandbox + permission land.
type codingToolGroup struct {
	tools []chat.Tool
}

func (g *codingToolGroup) Metadata() core.ToolGroupMetadata {
	return core.SimpleToolGroupMetadata{RoleText: ToolRoleCoding}
}

func (g *codingToolGroup) Tools(_ context.Context) ([]core.AgentTool, error) {
	return g.tools, nil
}
