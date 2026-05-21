package engine

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/tools/bash"
	"github.com/Tangerg/lynx/tools/fs"
)

// ToolRoleCoding is the role name the chat agent declares to require
// the default coding tool group. Action bodies that opt into this
// role get every tool returned by [BuildCodingTools] wired into their
// chat request.
const ToolRoleCoding = "coding"

// BuildCodingTools returns the default tool set Lyra ships with —
// read / write / edit / glob / grep / bash. workdir constrains every
// file operation to that root (empty string = unconfined; useful for
// tests but not for production).
//
// Provider-backed tools (webfetch / websearch / httpreq) are NOT
// included here — they need configured providers and arrive in
// future milestones once Lyra's config grows API-key plumbing.
func BuildCodingTools(workdir string) []chat.Tool {
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

// buildCodingResolver wires the coding tool set behind the
// [ToolRoleCoding] role on a fresh [core.StaticToolGroupResolver].
// The resolver is registered as a platform-scope extension so every
// agent (just the chat agent for now) can opt-in via [core.ToolRolesFor].
func buildCodingResolver(workdir string) *core.StaticToolGroupResolver {
	tools := BuildCodingTools(workdir)
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
