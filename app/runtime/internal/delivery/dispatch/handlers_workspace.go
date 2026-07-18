package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ─── Workspace (API.md §7.5) ────────────────────────────────────────
//
// Read methods take an optional cwd (default = serve dir); MCP methods
// are runtime-global and take no cwd. All list results are Page[T] (§4.11).

func (d *Dispatcher) handleWorkspaceListFileChanges(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListFileChanges(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceGetDiff(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GetDiffRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceGetDiff(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceGetFileHead(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GetFileHeadRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Path == "" {
		return responseError(msg.ID, invalidParams("path is required"))
	}
	out, err := d.api.WorkspaceGetFileHead(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListFiles(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListFilesRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListFiles(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceReadFile(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ReadFileRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Path == "" {
		return responseError(msg.ID, invalidParams("path is required"))
	}
	out, err := d.api.WorkspaceReadFile(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceGrep(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GrepRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Query == "" {
		return responseError(msg.ID, invalidParams("query is required"))
	}
	out, err := d.api.WorkspaceGrep(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListProjects(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListProjects(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListSkills(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListSkills(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListManagedSkills(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListManagedSkills(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceArchiveSkill(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SkillNameRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.WorkspaceArchiveSkill(ctx, in))
}

func (d *Dispatcher) handleWorkspaceRestoreSkill(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SkillNameRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.WorkspaceRestoreSkill(ctx, in))
}

func (d *Dispatcher) handleWorkspaceListRecipes(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListRecipes(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListAgentDocs(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListAgentDocs(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPListServers(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceMCPListServers(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPListTools(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.MCPListToolsRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceMCPListTools(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPReconnect(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.MCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Server == "" {
		return responseError(msg.ID, invalidParams("server is required"))
	}
	return replyDone(msg, d.api.WorkspaceMCPReconnect(ctx, in.Server))
}

func (d *Dispatcher) handleWorkspaceMCPAuthorize(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.MCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Server == "" {
		return responseError(msg.ID, invalidParams("server is required"))
	}
	return replyDone(msg, d.api.WorkspaceMCPAuthorize(ctx, in.Server))
}

func (d *Dispatcher) handleWorkspaceMCPListConfigs(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceMCPListConfigs(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPConfigure(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ConfigureMCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	out, err := d.api.WorkspaceMCPConfigure(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPRemove(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.RemoveMCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.WorkspaceMCPRemove(ctx, in.Name))
}

func (d *Dispatcher) handleWorkspaceMCPSetEnabled(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SetMCPEnabledRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.WorkspaceMCPSetEnabled(ctx, in))
}

func (d *Dispatcher) handleWorkspaceMCPTest(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ConfigureMCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceMCPTest(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListHooks(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListHooksRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.WorkspaceListHooks(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceSetHookTrust(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SetHookTrustRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ProjectRoot == "" {
		return responseError(msg.ID, invalidParams("projectRoot is required"))
	}
	return replyDone(msg, d.api.WorkspaceSetHookTrust(ctx, in))
}

// handleWorkspaceSubscribe opens the workspace event stream (AUX_API §3.1) and
// adapts its WorkspaceEvents into ephemeral StreamFrames.
func (d *Dispatcher) handleWorkspaceSubscribe(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceSubscribeRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, events, err := d.api.WorkspaceSubscribe(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, adaptStream(ctx, events, workspaceEventToFrame))
}
