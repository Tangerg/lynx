package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// ─── Workspace ──────────────────────────────────────────────────────
//
// Non-paginated list methods return bare arrays (no {items} wrapper)
// per API.md v3 §5.2.

func (d *Dispatcher) handleWorkspaceFilesChanged(ctx context.Context, msg *transport.Request) HandleResult {
	files, err := d.api.WorkspaceFilesChanged(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, files)
}

func (d *Dispatcher) handleWorkspaceDiff(ctx context.Context, msg *transport.Request) HandleResult {
	var in struct {
		Path string `json:"path"`
	}
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	rows, err := d.api.WorkspaceDiff(ctx, in.Path)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, rows)
}

func (d *Dispatcher) handleWorkspaceFileHead(ctx context.Context, msg *transport.Request) HandleResult {
	var in struct {
		Path string `json:"path"`
	}
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	lines, err := d.api.WorkspaceFileHead(ctx, in.Path)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, lines)
}

func (d *Dispatcher) handleWorkspaceGrep(ctx context.Context, msg *transport.Request) HandleResult {
	var in struct {
		Query string `json:"query"`
	}
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	res, err := d.api.WorkspaceGrep(ctx, in.Query)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, res)
}

func (d *Dispatcher) handleWorkspaceProjects(ctx context.Context, msg *transport.Request) HandleResult {
	projects, err := d.api.WorkspaceProjects(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, projects)
}

func (d *Dispatcher) handleWorkspaceMCPList(ctx context.Context, msg *transport.Request) HandleResult {
	servers, err := d.api.WorkspaceMCPList(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, servers)
}

func (d *Dispatcher) handleWorkspaceMCPReconnect(ctx context.Context, msg *transport.Request) HandleResult {
	var in struct {
		Name string `json:"name"`
	}
	if err := unmarshal(msg.Params, &in); err != nil {
		return responseError(msg.ID, invalidParams(err.Error()))
	}
	if err := d.api.WorkspaceMCPReconnect(ctx, in.Name); err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

func (d *Dispatcher) handleWorkspaceSkills(ctx context.Context, msg *transport.Request) HandleResult {
	skills, err := d.api.WorkspaceSkills(ctx)
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, skills)
}
