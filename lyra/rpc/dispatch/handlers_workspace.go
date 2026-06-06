package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Workspace (API.md §7.5) ────────────────────────────────────────
//
// Read methods take an optional cwd (default = serve dir); MCP methods
// are runtime-global and take no cwd. All list results are Page[T] (§4.11).

func (d *Dispatcher) handleWorkspaceListFileChanges(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.WorkspaceListQuery
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceListFileChanges(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceGetDiff(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.GetDiffRequest
	_ = unmarshal(msg.Params, &in)
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
	var q protocol.PageQuery
	_ = unmarshal(msg.Params, &q)
	out, err := d.api.WorkspaceListProjects(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListSkills(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.WorkspaceListQuery
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceListSkills(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceListAgentDocs(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.WorkspaceListQuery
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceListAgentDocs(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPListServers(ctx context.Context, msg *transport.Request) HandleResult {
	var q protocol.PageQuery
	_ = unmarshal(msg.Params, &q)
	out, err := d.api.WorkspaceMCPListServers(ctx, q)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPListTools(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.MCPListToolsRequest
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceMCPListTools(ctx, in)
	return reply(msg, out, err)
}

func (d *Dispatcher) handleWorkspaceMCPReconnect(ctx context.Context, msg *transport.Request) HandleResult {
	server, err := decodeStringParam(msg.Params, "server")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	return replyDone(msg, d.api.WorkspaceMCPReconnect(ctx, server))
}
