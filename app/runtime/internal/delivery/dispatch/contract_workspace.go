package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerWorkspace(r *Registry) {
	Query(r, MethodMeta{
		Name:      "workspaces.resolve",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
		return d.api.ResolveWorkspace(ctx, in)
	})

	Query(r, MethodMeta{Name: "workspaces.list", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.WorkspaceSummary], error) {
			return d.api.ListWorkspaces(ctx)
		})

	// Git reads require the advertised capability. Once negotiated, a path that is
	// not a repository is the distinct vcs_unavailable domain answer.
	Query(r, MethodMeta{
		Name: "workspace.changes.list",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
		return d.api.ListWorkspaceFileChanges(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.diff.get",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.GetDiffRequest) (*protocol.Diff, error) {
		return d.api.GetWorkspaceDiff(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.head",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
		return d.api.GetWorkspaceFileHead(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.search",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GrepRequest) (*protocol.GrepResult, error) {
		return d.api.GrepWorkspace(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.list",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
		return d.api.ListWorkspaceFiles(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.read",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ReadFileRequest) (*protocol.FileContent, error) {
		return d.api.ReadWorkspaceFile(ctx, in)
	})
}
