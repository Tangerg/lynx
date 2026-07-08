package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

type hookInspector interface {
	Inspect(ctx context.Context, cwd string) hooks.Inspection
}

// InspectHooks returns the lifecycle hooks discovered for cwd plus the project's
// trust status (workspace.hooks.list). Empty when hooks are unconfigured.
func (r *Runtime) InspectHooks(ctx context.Context, cwd string) hooks.Inspection {
	if r.hookInspection == nil {
		return hooks.Inspection{}
	}
	return r.hookInspection.Inspect(ctx, cwd)
}

// SetProjectHookTrust trusts (or revokes) a project's hooks (workspace.hooks.
// setTrust). No-op when no trust store is wired.
func (r *Runtime) SetProjectHookTrust(ctx context.Context, projectRoot string, trusted bool) error {
	if r.hookTrust == nil {
		return nil
	}
	if trusted {
		return r.hookTrust.Trust(ctx, projectRoot)
	}
	return r.hookTrust.Untrust(ctx, projectRoot)
}
