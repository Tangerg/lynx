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
	out, err := d.api.ListWorkspaceFileChanges(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceGetDiff(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.GetDiffRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.GetWorkspaceDiff(ctx, in)
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
	out, err := d.api.GetWorkspaceFileHead(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListFiles(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListFilesRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListWorkspaceFiles(ctx, in)
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
	out, err := d.api.ReadWorkspaceFile(ctx, in)
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
	out, err := d.api.GrepWorkspace(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListProjects(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListWorkspaceProjects(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSkillsDiscoveredList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListDiscoveredSkills(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSkillsLibraryList(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListManagedSkills(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSkillsLibraryArchive(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SkillNameRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.ArchiveSkill(ctx, in))
}

func (d *Dispatcher) handleSkillsLibraryRestore(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SkillNameRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.RestoreSkill(ctx, in))
}

func (d *Dispatcher) handleSkillsDraftsList(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListSkillDrafts(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleSkillsDraftsPromote(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SkillDraftRef](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	return replyDone(msg, d.api.PromoteSkillDraft(ctx, in))
}

func (d *Dispatcher) handleSkillsDraftsReject(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SkillDraftRef](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	return replyDone(msg, d.api.RejectSkillDraft(ctx, in))
}

func (d *Dispatcher) handleRecipesList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListRecipes(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleAgentDocsList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceListQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListAgentDocs(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleMCPServersList(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListMCPServers(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleMCPToolsList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.MCPListToolsRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListMCPTools(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleMCPServersReconnect(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.MCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Server == "" {
		return responseError(msg.ID, invalidParams("server is required"))
	}
	return replyDone(msg, d.api.ReconnectMCPServer(ctx, in.Server))
}

func (d *Dispatcher) handleMCPServersAuthorize(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.MCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Server == "" {
		return responseError(msg.ID, invalidParams("server is required"))
	}
	return replyDone(msg, d.api.AuthorizeMCPServer(ctx, in.Server))
}

func (d *Dispatcher) handleMCPConfigsList(ctx context.Context, msg *transport.Request) HandleResult {
	q, bad := decode[protocol.PageQuery](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListMCPServerConfigs(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleMCPConfigsConfigure(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ConfigureMCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	out, err := d.api.ConfigureMCPServer(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleMCPConfigsRemove(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.RemoveMCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.RemoveMCPServer(ctx, in.Name))
}

func (d *Dispatcher) handleMCPConfigsSetEnabled(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SetMCPEnabledRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.Name == "" {
		return responseError(msg.ID, invalidParams("name is required"))
	}
	return replyDone(msg, d.api.SetMCPServerEnabled(ctx, in))
}

func (d *Dispatcher) handleMCPConfigsTest(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ConfigureMCPServerRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.TestMCPServer(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleHooksList(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.ListHooksRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, err := d.api.ListHooks(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleHooksSetTrust(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.SetHookTrustRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	if in.ProjectRoot == "" {
		return responseError(msg.ID, invalidParams("projectRoot is required"))
	}
	return replyDone(msg, d.api.SetHookTrust(ctx, in))
}

// handleWorkspaceSubscribe opens the workspace event stream (AUX_API §3.1) and
// adapts its WorkspaceEvents into ephemeral StreamFrames.
func (d *Dispatcher) handleWorkspaceSubscribe(ctx context.Context, msg *transport.Request) HandleResult {
	in, bad := decode[protocol.WorkspaceSubscribeRequest](msg)
	if bad != nil {
		return responseError(msg.ID, bad)
	}
	out, events, err := d.api.SubscribeWorkspace(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, adaptStream(ctx, events, workspaceEventToFrame))
}
