package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/hookpolicy"
)

type hookBinding interface {
	ListHooks(context.Context, protocol.ListHooksRequest, embedded.CallOptions) (*protocol.HooksListResult, error)
	SetHookTrust(context.Context, protocol.SetHookTrustRequest, embedded.CommandOptions) error
}

type hookAdapter struct{ runtime *Runtime }

var _ hookpolicy.Service = (*hookAdapter)(nil)

func (adapter *hookAdapter) Catalog(ctx context.Context, workspace string) (hookpolicy.Catalog, error) {
	r := adapter.runtime
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return hookpolicy.Catalog{}, errors.New("list hooks: workspace is empty")
	}
	result, err := r.hooks.ListHooks(ctx, protocol.ListHooksRequest{
		Workspace: protocol.WorkspaceRef{Path: workspace},
	}, r.callOptions())
	if err != nil {
		return hookpolicy.Catalog{}, classifyError(err)
	}
	if result == nil {
		return hookpolicy.Catalog{}, errors.New("list hooks: runtime returned nil")
	}
	catalog := hookpolicy.Catalog{
		ProjectRoot: result.ProjectRoot, ProjectTrusted: result.ProjectTrusted,
		Hooks: make([]hookpolicy.Hook, 0, len(result.Hooks)),
	}
	for _, value := range result.Hooks {
		catalog.Hooks = append(catalog.Hooks, hookpolicy.Hook{
			Event: hookpolicy.Event(value.Event), Matcher: value.Matcher,
			Command: value.Command, Inject: value.Inject, TimeoutMillis: value.TimeoutMillis,
			Scope: hookpolicy.Scope(value.Scope), Source: value.Source, Active: value.Active,
		})
	}
	if err := catalog.Validate(); err != nil {
		return hookpolicy.Catalog{}, fmt.Errorf("list hooks: %w", err)
	}
	return catalog, nil
}

func (adapter *hookAdapter) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	r := adapter.runtime
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return errors.New("set hook trust: project root is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.hooks.SetHookTrust(ctx, protocol.SetHookTrustRequest{
		ProjectRoot: projectRoot, Trusted: trusted,
	}, options))
}
