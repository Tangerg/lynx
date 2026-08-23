// Package hookflow owns lifecycle-hook discovery, project trust, and effective
// activation. Execution remains a separate Run application concern; the
// protocol sees a reviewable projection of the same domain values Runs use.
package hookflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type Store interface {
	ProjectHookTrusted(context.Context, string) (bool, error)
	SetProjectHookTrusted(context.Context, string, bool, time.Time) (bool, error)
}

type Source interface {
	Global(context.Context) ([]lifecyclehook.Hook, error)
	Project(context.Context, lifecyclehook.Target) ([]lifecyclehook.Hook, error)
	Files(context.Context, []lifecyclehook.Target) ([]string, error)
}

type Resolver interface {
	Resolve(context.Context, string) (workspacefs.Resolution, error)
}

type Config struct {
	Store    Store
	Source   Source
	Resolver Resolver
	Clock    func() time.Time
}

type Service struct {
	store    Store
	source   Source
	resolver Resolver
	now      func() time.Time
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.Source == nil || config.Resolver == nil {
		return nil, errors.New("hookflow: store, source, and resolver are required")
	}
	now := config.Clock
	if now == nil {
		now = time.Now
	}
	return &Service{
		store: config.Store, source: config.Source,
		resolver: config.Resolver, now: now,
	}, nil
}

func (service *Service) List(
	ctx context.Context,
	request protocol.ListHooksRequest,
) (*protocol.HooksListResult, error) {
	target, err := service.resolve(ctx, request.Workspace.Path)
	if err != nil {
		return nil, err
	}
	trusted, err := service.store.ProjectHookTrusted(ctx, target.ProjectRoot)
	if err != nil {
		return nil, err
	}
	global, err := service.source.Global(ctx)
	if err != nil {
		return nil, err
	}
	project, err := service.source.Project(ctx, target)
	if err != nil {
		return nil, err
	}
	values, err := combine(global, project)
	if err != nil {
		return nil, err
	}
	hooks := make([]protocol.HookInfo, len(values))
	for index, hook := range values {
		hooks[index] = present(hook, trusted)
	}
	return &protocol.HooksListResult{
		ProjectRoot: target.ProjectRoot, ProjectTrusted: trusted, Hooks: hooks,
	}, nil
}

func (service *Service) SetTrust(
	ctx context.Context,
	request protocol.SetHookTrustRequest,
) (bool, error) {
	project := filepath.Clean(request.ProjectRoot)
	if !filepath.IsAbs(request.ProjectRoot) || project != request.ProjectRoot {
		return false, fmt.Errorf("%w: project root must be canonical and absolute", protocol.ErrInvalidParams)
	}
	target, err := service.resolve(ctx, project)
	if err != nil {
		return false, err
	}
	if target.ProjectRoot != project {
		return false, fmt.Errorf("%w: path is not the resolved project root", protocol.ErrInvalidParams)
	}
	changed, err := service.store.SetProjectHookTrusted(
		ctx,
		project,
		request.Trusted,
		service.now().UTC(),
	)
	if err != nil {
		return false, err
	}
	return changed, nil
}

// Active resolves the execution set. An untrusted project's files are not
// opened, so hostile malformed JSON cannot disable valid global hooks.
func (service *Service) Active(
	ctx context.Context,
	workspace string,
) ([]lifecyclehook.Hook, error) {
	target, err := service.resolve(ctx, workspace)
	if err != nil {
		return nil, err
	}
	global, err := service.source.Global(ctx)
	if err != nil {
		return nil, err
	}
	trusted, err := service.store.ProjectHookTrusted(ctx, target.ProjectRoot)
	if err != nil {
		return nil, err
	}
	if !trusted {
		return global, nil
	}
	project, err := service.source.Project(ctx, target)
	if err != nil {
		return nil, err
	}
	return combine(global, project)
}

func (service *Service) HookFiles(
	ctx context.Context,
	workspaces []protocol.WorkspaceRef,
) ([]string, error) {
	targets := make([]lifecyclehook.Target, 0, len(workspaces))
	for _, workspace := range workspaces {
		target, err := service.resolve(ctx, workspace.Path)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
	}
	return service.source.Files(ctx, targets)
}

func (service *Service) resolve(
	ctx context.Context,
	workspace string,
) (lifecyclehook.Target, error) {
	resolved, err := service.resolver.Resolve(ctx, workspace)
	if err != nil || !resolved.Available {
		if cancellation := ctx.Err(); cancellation != nil {
			return lifecyclehook.Target{}, cancellation
		}
		return lifecyclehook.Target{}, protocol.ErrWorkspaceUnavailable
	}
	root := resolved.ProjectRoot
	if root == "" {
		root = resolved.Workspace.Path()
	}
	target := lifecyclehook.Target{
		ProjectRoot: root,
		Workspace: resolved.Workspace.Path(),
	}
	if err := target.Validate(); err != nil {
		return lifecyclehook.Target{}, err
	}
	return target, nil
}

func combine(
	global []lifecyclehook.Hook,
	project []lifecyclehook.Hook,
) ([]lifecyclehook.Hook, error) {
	if len(global)+len(project) > lifecyclehook.MaxHooksPerRun {
		return nil, fmt.Errorf(
			"hookflow: hook cascade exceeds %d entries",
			lifecyclehook.MaxHooksPerRun,
		)
	}
	values := slices.Clone(global)
	return append(values, project...), nil
}

func present(hook lifecyclehook.Hook, projectTrusted bool) protocol.HookInfo {
	return protocol.HookInfo{
		Event: protocol.HookEvent(hook.Event), Matcher: hook.Matcher,
		Command: hook.Command, Inject: hook.Inject,
		TimeoutMillis: hook.TimeoutMillis,
		Scope: protocol.HookScope(hook.Scope), Source: hook.Source,
		Active: hook.Scope == lifecyclehook.ScopeGlobal || projectTrusted,
	}
}
