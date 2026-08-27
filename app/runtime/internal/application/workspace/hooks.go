package workspace

import (
	"context"

	apphooks "github.com/Tangerg/scope/app/runtime/internal/application/hooks"
	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/hooks"
)

// HookInspector resolves lifecycle hooks and project trust for a working directory.
type HookInspector interface {
	Inspect(ctx context.Context, cwd string) (apphooks.Inspection, error)
}

// HookTrustStore mutates project hook trust. nil leaves trust read-only.
type HookTrustStore interface {
	Trust(ctx context.Context, projectRoot string) error
	Untrust(ctx context.Context, projectRoot string) error
}

// Hooks owns lifecycle-hook inspection and trust decisions.
type Hooks struct {
	scope         *Scope
	inspector     HookInspector
	trust         HookTrustStore
	invalidations invalidation.Publish
}

// HookInspection is the resolved hook view after applying trust policy.
type HookInspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []ResolvedHook
}

type ResolvedHook struct {
	Hook   hooks.Hook
	Active bool
}

func NewHooks(scope *Scope, inspector HookInspector, trust HookTrustStore, invalidations invalidation.Publish) *Hooks {
	return &Hooks{scope: scope, inspector: inspector, trust: trust, invalidations: invalidations}
}

// Inspect returns lifecycle hooks and their effective activation state.
func (h *Hooks) Inspect(ctx context.Context, cwd string) (HookInspection, error) {
	root, err := h.scope.root(cwd)
	if err != nil {
		return HookInspection{}, err
	}
	if h.inspector == nil {
		return HookInspection{}, nil
	}
	inspection, err := h.inspector.Inspect(ctx, root)
	if err != nil {
		return HookInspection{}, err
	}
	resolved := HookInspection{
		ProjectRoot: inspection.ProjectRoot, ProjectTrusted: inspection.ProjectTrusted,
		Hooks: make([]ResolvedHook, 0, len(inspection.Hooks)),
	}
	for _, hook := range inspection.Hooks {
		resolved.Hooks = append(resolved.Hooks, ResolvedHook{
			Hook: hook, Active: hook.Scope == hooks.ScopeGlobal || inspection.ProjectTrusted,
		})
	}
	return resolved, nil
}

// SetProjectTrust changes whether project hooks may run.
func (h *Hooks) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	root, err := h.scope.root(projectRoot)
	if err != nil {
		return err
	}
	if h.trust == nil {
		return nil
	}
	var changeErr error
	if trusted {
		changeErr = h.trust.Trust(ctx, root)
	} else {
		changeErr = h.trust.Untrust(ctx, root)
	}
	if changeErr == nil {
		h.invalidations.Notify(invalidation.Notice{Resource: invalidation.Hooks})
	}
	return changeErr
}
