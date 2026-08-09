package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerWorkspace(registry *Registry) {
	Query(registry, MethodMeta{
		Name:      "workspaces.resolve",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
		return router.api.ResolveWorkspace(ctx, request)
	})

	Query(registry, MethodMeta{Name: "workspaces.list", Stability: stable},
		func(router *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.WorkspaceSummary], error) {
			return router.api.ListWorkspaces(ctx)
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
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
		return router.api.ListWorkspaceFileChanges(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.diff.get",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.GetDiffRequest) (*protocol.Diff, error) {
		return router.api.GetWorkspaceDiff(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.head",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
		return router.api.GetWorkspaceFileHead(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.search",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.GrepRequest) (*protocol.GrepResult, error) {
		return router.api.GrepWorkspace(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.list",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
		return router.api.ListWorkspaceFiles(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "workspace.files.read",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.ReadFileRequest) (*protocol.FileContent, error) {
		return router.api.ReadWorkspaceFile(ctx, request)
	})
}
