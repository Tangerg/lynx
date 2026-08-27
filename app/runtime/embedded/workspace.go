package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ResolveWorkspace resolves and validates a workspace reference.
func (r *Runtime) ResolveWorkspace(ctx context.Context, request protocol.ResolveWorkspaceRequest, options CallOptions) (*protocol.WorkspaceInfo, error) {
	return r.invoke[protocol.ResolveWorkspaceRequest, *protocol.WorkspaceInfo](ctx, operation.WorkspacesResolve, request, callOptions(options))
}

// ListWorkspaces returns the Runtime's discoverable workspaces.
func (r *Runtime) ListWorkspaces(ctx context.Context, options CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.WorkspaceSummary]](ctx, operation.WorkspacesList, struct{}{}, callOptions(options))
}

// ListWorkspaceFileChanges returns version-control changes in a workspace.
func (r *Runtime) ListWorkspaceFileChanges(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.WorkspaceFileChange]](ctx, operation.WorkspaceChangesList, request, callOptions(options))
}

// GetWorkspaceDiff returns a workspace diff.
func (r *Runtime) GetWorkspaceDiff(ctx context.Context, request protocol.GetDiffRequest, options CallOptions) (*protocol.Diff, error) {
	return r.invoke[protocol.GetDiffRequest, *protocol.Diff](ctx, operation.WorkspaceDiffGet, request, callOptions(options))
}

// GetWorkspaceFileHead returns metadata for one workspace file.
func (r *Runtime) GetWorkspaceFileHead(ctx context.Context, request protocol.GetFileHeadRequest, options CallOptions) (*protocol.FileHead, error) {
	return r.invoke[protocol.GetFileHeadRequest, *protocol.FileHead](ctx, operation.WorkspaceFilesHead, request, callOptions(options))
}

// SearchWorkspaceFiles searches text within a workspace.
func (r *Runtime) SearchWorkspaceFiles(ctx context.Context, request protocol.GrepRequest, options CallOptions) (*protocol.GrepResult, error) {
	return r.invoke[protocol.GrepRequest, *protocol.GrepResult](ctx, operation.WorkspaceFilesSearch, request, callOptions(options))
}

// ListWorkspaceFiles returns one cursor page of workspace entries.
func (r *Runtime) ListWorkspaceFiles(ctx context.Context, request protocol.ListFilesRequest, options CallOptions) (*protocol.Page[protocol.FileEntry], error) {
	return r.invoke[protocol.ListFilesRequest, *protocol.Page[protocol.FileEntry]](ctx, operation.WorkspaceFilesList, request, callOptions(options))
}

// ReadWorkspaceFile reads one workspace file.
func (r *Runtime) ReadWorkspaceFile(ctx context.Context, request protocol.ReadFileRequest, options CallOptions) (*protocol.FileContent, error) {
	return r.invoke[protocol.ReadFileRequest, *protocol.FileContent](ctx, operation.WorkspaceFilesRead, request, callOptions(options))
}
