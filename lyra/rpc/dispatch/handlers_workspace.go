package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Workspace (API.md §7.5) ────────────────────────────────────────
//
// Read methods take an optional cwd (default = serve dir); MCP methods
// are runtime-global and take no cwd. All list results are bare arrays.

func (d *Dispatcher) handleWorkspaceListFileChanges(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.WorkspaceQuery
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceListFileChanges(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceGetDiff(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.GetDiffRequest
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceGetDiff(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceGetFileHead(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.GetFileHeadRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.Path == "" {
		return responseError(msg.ID, invalidParams("path is required"))
	}
	out, err := d.api.WorkspaceGetFileHead(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceGrep(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.GrepRequest
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if in.Query == "" {
		return responseError(msg.ID, invalidParams("query is required"))
	}
	out, err := d.api.WorkspaceGrep(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceListProjects(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.WorkspaceListProjects(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceListSkills(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.WorkspaceQuery
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceListSkills(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceListAgentDocs(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.WorkspaceQuery
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceListAgentDocs(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceMCPListServers(ctx context.Context, msg *transport.Request) HandleResult {
	out, err := d.api.WorkspaceMCPListServers(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceMCPListTools(ctx context.Context, msg *transport.Request) HandleResult {
	var in protocol.MCPListToolsRequest
	_ = unmarshal(msg.Params, &in)
	out, err := d.api.WorkspaceMCPListTools(ctx, in)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

func (d *Dispatcher) handleWorkspaceMCPReconnect(ctx context.Context, msg *transport.Request) HandleResult {
	server, err := decodeStringParam(msg.Params, "server")
	if err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.WorkspaceMCPReconnect(ctx, server); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}
