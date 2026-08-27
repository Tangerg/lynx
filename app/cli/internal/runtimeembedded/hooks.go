package runtimeembedded

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/hookpolicy"
)

type hookBinding interface {
	ListHooks(context.Context, protocol.ListHooksRequest, embedded.CallOptions) (*protocol.HooksListResult, error)
	SetHookTrust(context.Context, protocol.SetHookTrustRequest, embedded.CommandOptions) error
}

type hookAdapter struct{ runtime *Runtime }

var _ hookpolicy.Service = (*hookAdapter)(nil)

func (h *hookAdapter) Catalog(ctx context.Context, workspace string) (hookpolicy.Catalog, error) {
	r := h.runtime
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return hookpolicy.Catalog{}, errors.New("list hooks: workspace is empty")
	}
	if !filepath.IsAbs(workspace) {
		return hookpolicy.Catalog{}, errors.New("list hooks: workspace is not absolute")
	}
	result, err := r.hooks.ListHooks(ctx, protocol.ListHooksRequest{
		Workspace: protocol.WorkspaceRef{Path: workspace},
	}, r.callOptions())
	if err != nil {
		return hookpolicy.Catalog{}, classifyError(err)
	}
	if result == nil {
		return hookpolicy.Catalog{}, runtimeContractViolation("list hooks returned nil")
	}
	if !hookProjectRootContainsWorkspace(result.ProjectRoot, workspace) {
		return hookpolicy.Catalog{}, runtimeContractViolation(
			"list hooks for workspace %q returned unrelated project root %q",
			workspace,
			result.ProjectRoot,
		)
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
		return hookpolicy.Catalog{}, runtimeContractViolation("list hooks returned an invalid catalog: %v", err)
	}
	return catalog, nil
}

func hookProjectRootContainsWorkspace(projectRoot, workspace string) bool {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" || !filepath.IsAbs(projectRoot) {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(projectRoot), filepath.Clean(workspace))
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (h *hookAdapter) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	r := h.runtime
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
