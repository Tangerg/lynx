package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ResolveWorkspace resolves and validates a workspace reference.
func (r *Runtime) ResolveWorkspace(ctx context.Context, request protocol.ResolveWorkspaceRequest, options CallOptions) (*protocol.WorkspaceInfo, error) {
	return invoke[protocol.ResolveWorkspaceRequest, *protocol.WorkspaceInfo](ctx, r, "workspaces.resolve", request, callOptions(options))
}

// ListWorkspaces returns the Runtime's discoverable workspaces.
func (r *Runtime) ListWorkspaces(ctx context.Context, options CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error) {
	return invoke[struct{}, *protocol.Page[protocol.WorkspaceSummary]](ctx, r, "workspaces.list", struct{}{}, callOptions(options))
}

// ListWorkspaceFileChanges returns version-control changes in a workspace.
func (r *Runtime) ListWorkspaceFileChanges(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.WorkspaceFileChange]](ctx, r, "workspace.changes.list", request, callOptions(options))
}

// GetWorkspaceDiff returns a workspace diff.
func (r *Runtime) GetWorkspaceDiff(ctx context.Context, request protocol.GetDiffRequest, options CallOptions) (*protocol.Diff, error) {
	return invoke[protocol.GetDiffRequest, *protocol.Diff](ctx, r, "workspace.diff.get", request, callOptions(options))
}

// GetWorkspaceFileHead returns metadata for one workspace file.
func (r *Runtime) GetWorkspaceFileHead(ctx context.Context, request protocol.GetFileHeadRequest, options CallOptions) (*protocol.FileHead, error) {
	return invoke[protocol.GetFileHeadRequest, *protocol.FileHead](ctx, r, "workspace.files.head", request, callOptions(options))
}

// SearchWorkspaceFiles searches text within a workspace.
func (r *Runtime) SearchWorkspaceFiles(ctx context.Context, request protocol.GrepRequest, options CallOptions) (*protocol.GrepResult, error) {
	return invoke[protocol.GrepRequest, *protocol.GrepResult](ctx, r, "workspace.files.search", request, callOptions(options))
}

// ListWorkspaceFiles returns one cursor page of workspace entries.
func (r *Runtime) ListWorkspaceFiles(ctx context.Context, request protocol.ListFilesRequest, options CallOptions) (*protocol.Page[protocol.FileEntry], error) {
	return invoke[protocol.ListFilesRequest, *protocol.Page[protocol.FileEntry]](ctx, r, "workspace.files.list", request, callOptions(options))
}

// ReadWorkspaceFile reads one workspace file.
func (r *Runtime) ReadWorkspaceFile(ctx context.Context, request protocol.ReadFileRequest, options CallOptions) (*protocol.FileContent, error) {
	return invoke[protocol.ReadFileRequest, *protocol.FileContent](ctx, r, "workspace.files.read", request, callOptions(options))
}
