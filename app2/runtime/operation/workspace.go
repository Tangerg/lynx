package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerWorkspace(registry *Registry) {
	Query(registry, MethodMeta{
		Name:   "workspaces.resolve",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		ResolveWorkspace(context.Context, protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error)
	}, ctx context.Context, request protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
		return service.ResolveWorkspace(ctx, request)
	})

	Query(registry, MethodMeta{Name: "workspaces.list"},
		func(service interface {
			ListWorkspaces(context.Context) (*protocol.Page[protocol.WorkspaceSummary], error)
		}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.WorkspaceSummary], error) {
			return service.ListWorkspaces(ctx)
		})

	// Git reads require the advertised capability. Once negotiated, a path that is
	// not a repository is the distinct vcs_unavailable domain answer.
	Query(registry, MethodMeta{
		Name: "workspace.changes.list",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
	}, func(service interface {
		ListWorkspaceFileChanges(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
		return service.ListWorkspaceFileChanges(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.diff.get",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
	}, func(service interface {
		GetWorkspaceDiff(context.Context, protocol.GetDiffRequest) (*protocol.Diff, error)
	}, ctx context.Context, request protocol.GetDiffRequest) (*protocol.Diff, error) {
		return service.GetWorkspaceDiff(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.head",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, func(service interface {
		GetWorkspaceFileHead(context.Context, protocol.GetFileHeadRequest) (*protocol.FileHead, error)
	}, ctx context.Context, request protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
		return service.GetWorkspaceFileHead(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.search",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, func(service interface {
		GrepWorkspace(context.Context, protocol.GrepRequest) (*protocol.GrepResult, error)
	}, ctx context.Context, request protocol.GrepRequest) (*protocol.GrepResult, error) {
		return service.GrepWorkspace(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.list",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, func(service interface {
		ListWorkspaceFiles(context.Context, protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error)
	}, ctx context.Context, request protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
		return service.ListWorkspaceFiles(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.read",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, func(service interface {
		ReadWorkspaceFile(context.Context, protocol.ReadFileRequest) (*protocol.FileContent, error)
	}, ctx context.Context, request protocol.ReadFileRequest) (*protocol.FileContent, error) {
		return service.ReadWorkspaceFile(ctx, request)
	})
}
