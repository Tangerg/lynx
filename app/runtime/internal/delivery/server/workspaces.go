package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ResolveWorkspace projects the application's current filesystem identity onto
// the canonical wire resource.
func (s *Server) ResolveWorkspace(_ context.Context, in protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
	path := ""
	if in.Ref != nil {
		path = in.Ref.Path
	}
	resolved, err := s.workspaceDiscovery.Resolve(path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := presentWorkspaceInfo(resolved.Path, resolved.ProjectRoot, resolved.Missing)
	return &out, nil
}

// ListWorkspaces projects the application-owned distinct-workspace view
// derived from user-facing sessions.
func (s *Server) ListWorkspaces(ctx context.Context) (*protocol.Page[protocol.WorkspaceSummary], error) {
	workspaces, err := s.workspaceDiscovery.Workspaces(ctx)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.WorkspaceSummary, 0, len(workspaces))
	for _, workspace := range workspaces {
		lastActiveAt := workspace.LastActiveAt
		out = append(out, protocol.WorkspaceSummary{
			Workspace: presentWorkspaceInfo(workspace.Path, workspace.ProjectRoot, workspace.Missing),
			Name:      workspace.Name, SessionCount: workspace.SessionCount, LastActiveAt: &lastActiveAt,
		})
	}
	return protocol.NewPage(out), nil
}

func presentWorkspaceInfo(path, projectRoot string, missing bool) protocol.WorkspaceInfo {
	availability := protocol.WorkspaceAvailable
	if missing {
		availability = protocol.WorkspaceMissing
	}
	return protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: projectRoot, Availability: availability,
	}
}
